---
settings:
  inline-schema:
    sections:
      - heading: "Symptoms"
        required: true
        aliases: ["Indicators"]
      - heading: "Diagnosis"
        required: true
        sections:
          - heading: "Step"
            required: true
            sections:
              - heading: "Check"
                required: true
              - heading: "Expected"
                required: true
              - heading: "If different"
                required: false
      - heading: "References"
        required: false
---
# Runbook

## Indicators

Listed as alias of Symptoms.

## Diagnosis

### Step

#### Check

Probe state.

#### Expected

Healthy.

## References

Doc link here.
