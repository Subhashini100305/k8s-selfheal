package contracts

import "time"

// DetectionEvent is emitted by the operator (Owner 1) the moment a Pod is
// confirmed to be in CrashLoopBackOff. Owner 2's safety layer consumes it.
type DetectionEvent struct {
	PodName         string
	Namespace       string
	ContainerName   string // which container is crash-looping — verify THIS one's restart count, not the pod as a whole
	RestartCount    int32
	OwnerDeployment string // empty if not Deployment-owned
	Timestamp       time.Time

	// IncidentID and AttemptNumber identify which remediation attempt this
	// event belongs to. Owner 1 sets both; Owner 2 copies them onto every
	// audit line so the metrics module can group transitions into attempts
	// and attempts into incidents (docs/measurement-definitions.md §1, §6).
	// An incident runs 1..MaxAttempts attempts; AttemptNumber is 1-based and
	// stays 0 for an incident that escalated or was rejected before any
	// action ran.
	IncidentID    string
	AttemptNumber int
}
