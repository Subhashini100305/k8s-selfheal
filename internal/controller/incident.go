package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aryausingh/k8s-selfheal/internal/safety"
)

// Attempt budget. These values appear in the report, so they are exported
// constants rather than literals — docs/measurement-definitions.md §5 is the
// source of truth and these must match it.
const (
	MaxAttempts        = 3
	AttemptBackoffBase = 30 * time.Second
	CooldownPeriod     = 5 * time.Minute
)

// Incident terminal outcomes. Note the distinction the measurement
// definitions depend on: an *attempt* ends recovered or rolled_back, an
// *incident* ends in one of these four. A rolled_back attempt does not end the
// incident — the next one retries under backoff until the budget is spent, at
// which point the incident ends exhausted.
const (
	OutcomeRecovered = "recovered"
	OutcomeExhausted = "exhausted"
	OutcomeEscalated = "escalated"
	OutcomeRejected  = "rejected"
)

// incidentRecord is the per-Deployment remediation record. One incident per
// Deployment at a time, keyed the same way the old in-flight guard was —
// "namespace/OwnerDeployment", never pod UID, because RestartPod deletes the
// pod and the replacement carries a brand-new UID.
type incidentRecord struct {
	id              string
	attemptCount    int
	firstDetectedAt time.Time
	lastAttemptAt   time.Time
	terminalOutcome string // empty while the incident is still active
	terminalAt      time.Time
	generation      int64 // Deployment metadata.generation last seen
	inFlight        bool  // an attempt is running right now
}

// admission is what beginAttempt tells Reconcile to do.
type admission int

const (
	admitProceed   admission = iota // claim taken, run an attempt
	admitSkip                       // in flight, backing off, or gone quiet
	admitExhausted                  // budget just spent — log once, then go quiet
)

// incidentKey builds the per-Deployment record key.
func incidentKey(namespace, ownerDeployment string) string {
	return namespace + "/" + ownerDeployment
}

// beginAttempt decides whether an attempt may run for this Deployment and, if
// so, claims the incident so no concurrent reconcile can start a second one.
//
// A plain map under a mutex replaced the sync.Map the presence-only guard
// used. sync.Map's advantage was LoadOrStore giving atomic claim-if-absent in
// one call, but an incident is read-modify-write state — budget, backoff and
// cooldown all have to be evaluated against the same snapshot and then written
// back — so the value needs a lock regardless. One lock around the whole
// decision is both simpler and the only way the checks stay consistent with
// each other.
func (r *PodReconciler) beginAttempt(key string, generation int64, now time.Time) (*incidentRecord, admission) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.incidents == nil {
		r.incidents = make(map[string]*incidentRecord)
	}

	record := r.incidents[key]

	// A changed generation means a human edited the Deployment or a new
	// rollout landed, so whatever we concluded about the old spec no longer
	// applies. Drop the record entirely and start fresh — this is the only
	// thing that clears an exhausted incident.
	if record != nil && record.terminalOutcome != "" && generation != record.generation {
		delete(r.incidents, key)
		record = nil
	}

	if record != nil && record.terminalOutcome != "" {
		// Exhausted is sticky, every other terminal outcome expires after the
		// cooldown. Letting exhausted expire would re-arm the controller on a
		// Deployment we already gave up on, which is precisely the unbounded
		// remediation loop the budget exists to stop — the plan's acceptance
		// test is that a permanently-broken Deployment is silent at minute 10,
		// and a 5-minute cooldown would have it acting again at minute 8.
		if record.terminalOutcome == OutcomeExhausted || now.Sub(record.terminalAt) < CooldownPeriod {
			return record, admitSkip
		}
		delete(r.incidents, key)
		record = nil
	}

	if record == nil {
		record = &incidentRecord{
			id:              string(uuid.NewUUID()),
			firstDetectedAt: now,
		}
		r.incidents[key] = record
	}
	record.generation = generation

	if record.inFlight {
		return record, admitSkip
	}
	if record.attemptCount >= MaxAttempts {
		record.terminalOutcome = OutcomeExhausted
		record.terminalAt = now
		return record, admitExhausted
	}
	if record.attemptCount > 0 && now.Sub(record.lastAttemptAt) < attemptBackoff(record.attemptCount) {
		return record, admitSkip
	}

	record.inFlight = true
	return record, admitProceed
}

// recordAttempt increments the attempt counter and reports the new number. It
// is deliberately separate from beginAttempt and called only once an action is
// about to be dispatched: an incident that escalates or is rejected takes no
// action at all, so it must not consume budget (measurement-definitions.md §2
// — those outcomes are terminal at attempt 0).
func (r *PodReconciler) recordAttempt(key string, now time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.incidents[key]
	if record == nil {
		return 0
	}
	record.attemptCount++
	record.lastAttemptAt = now
	return record.attemptCount
}

// endAttempt releases the in-flight claim and ends the incident when the
// outcome is terminal.
//
// rolled_back is NOT terminal for the incident, and neither is an attempt that
// errored (outcome ""): both leave the record active so the next reconcile
// retries under backoff, and the incident ends exhausted once the budget runs
// out. Anything else — recovered, escalated, rejected — ends it here.
func (r *PodReconciler) endAttempt(key, outcome string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.incidents[key]
	if record == nil {
		return
	}
	record.inFlight = false
	if outcome == "" || outcome == string(safety.OutcomeRolledBack) {
		return
	}
	record.terminalOutcome = outcome
	record.terminalAt = now
}

// attemptBackoff is how long to wait before the next attempt, given how many
// have already completed: 30s after the first, 60s after the second. The shift
// is bounded because MaxAttempts caps the input.
func attemptBackoff(completedAttempts int) time.Duration {
	return AttemptBackoffBase << (completedAttempts - 1)
}

// deploymentGeneration reads metadata.generation, which Kubernetes increments
// on every spec change. It is the signal that a human or a new rollout
// intervened, and the only thing that resurrects an exhausted incident.
func (r *PodReconciler) deploymentGeneration(ctx context.Context, namespace, name string) (int64, error) {
	var deployment appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &deployment); err != nil {
		return 0, err
	}
	return deployment.Generation, nil
}

// claimHeld reports whether an attempt is currently running for this key. Only
// used by tests, which cannot read the map without taking mu.
func (r *PodReconciler) claimHeld(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.incidents[key]
	return record != nil && record.inFlight
}
