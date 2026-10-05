import json
from pathlib import Path

try:
    import matplotlib.pyplot as plt
except ImportError:
    raise SystemExit(
        "matplotlib is not installed. Stop here and tell ChatGPT; do not install anything yet."
    )

ROOT = Path(__file__).resolve().parents[2]
RESULTS = ROOT / "week3-live-final-results.json"
OUT = ROOT / "docs" / "figures"

with RESULTS.open("r", encoding="utf-8") as f:
    data = json.load(f)

# ---------------------------------------------------------
# FIGURE 1: Recovery rate by experimental arm
# ---------------------------------------------------------

recovery = data["experimentRecovery"]

arms = [
    "A1\nW1 Enabled",
    "A2\nW1 Disabled",
    "B1\nW2 Enabled",
    "B2\nW2 Disabled",
    "C1\nW3 Enabled",
    "C2\nW3 Disabled",
]

rates = [
    recovery["W1"]["enabled"]["ratePercent"],
    recovery["W1"]["disabled"]["ratePercent"],
    recovery["W2"]["enabled"]["ratePercent"],
    recovery["W2"]["disabled"]["ratePercent"],
    recovery["W3"]["enabled"]["ratePercent"],
    recovery["W3"]["disabled"]["ratePercent"],
]

fig, ax = plt.subplots(figsize=(10, 6))
bars = ax.bar(arms, rates)

ax.set_title("SAGE-K8s Recovery Rate by Experimental Arm")
ax.set_ylabel("Recovery rate (%)")
ax.set_ylim(0, 110)
ax.grid(axis="y", alpha=0.25)

for bar, value in zip(bars, rates):
    ax.text(
        bar.get_x() + bar.get_width() / 2,
        value + 2,
        f"{value:.0f}%",
        ha="center",
        va="bottom",
        fontweight="bold",
    )

fig.text(
    0.5,
    0.01,
    "W1 attributable recovery: 0 pp   |   "
    "W2: +100 pp   |   W3: 0 pp",
    ha="center",
)

fig.tight_layout(rect=[0, 0.05, 1, 1])

figure1 = OUT / "week3-recovery-by-arm.png"
fig.savefig(figure1, dpi=300, bbox_inches="tight")
plt.close(fig)

# ---------------------------------------------------------
# FIGURE 2: Average TTM decomposition
# ---------------------------------------------------------

timing = data["timingSeconds"]

ttm = timing["averageTTM"]
ttd = timing["averageTTD"]
classifier = timing["averageClassifier"]
apply_time = timing["averageApply"]
verify = timing["averageVerify"]

known = ttd + classifier + apply_time + verify
other = max(0.0, ttm - known)

components = [
    ("Detection", ttd),
    ("Classifier", classifier),
    ("Apply", apply_time),
    ("Verification", verify),
    ("Other lifecycle / retry", other),
]

fig, ax = plt.subplots(figsize=(10, 5))

left = 0.0
for label, value in components:
    ax.barh(
        ["Average TTM"],
        [value],
        left=left,
        label=f"{label} ({value:.2f}s)",
    )
    left += value

ax.set_title("SAGE-K8s Average Time-to-Mitigation (TTM) Breakdown")
ax.set_xlabel("Time (seconds)")
ax.set_xlim(0, ttm * 1.05)
ax.grid(axis="x", alpha=0.25)

ax.text(
    ttm,
    0,
    f"  {ttm:.2f}s total",
    va="center",
    fontweight="bold",
)

ax.legend(
    loc="upper center",
    bbox_to_anchor=(0.5, -0.20),
    ncol=2,
)

fig.text(
    0.5,
    0.01,
    "Other lifecycle / retry = TTM minus the separately measured "
    "detection, classifier, apply, and verification averages.",
    ha="center",
    fontsize=9,
)

fig.tight_layout(rect=[0, 0.10, 1, 1])

figure2 = OUT / "week3-ttm-decomposition.png"
fig.savefig(figure2, dpi=300, bbox_inches="tight")
plt.close(fig)

print("Generated:")
print(figure1)
print(figure2)
print()
print("Timing used:")
print(f"TTD        = {ttd:.3f}s")
print(f"Classifier = {classifier:.3f}s")
print(f"Apply      = {apply_time:.3f}s")
print(f"Verify     = {verify:.3f}s")
print(f"Other      = {other:.3f}s")
print(f"TTM total  = {ttm:.3f}s")
