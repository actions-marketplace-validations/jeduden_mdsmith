---
settings:
  inline-schema:
    sections:
      - heading: "Diagnosis"
        required: true
        sections:
          - heading: "Step"
            required: true
diagnostics:
  - line: 5
    column: 1
    message: 'heading level mismatch for "Step": expected h3, got h2'
---
# Runbook

## Diagnosis

## Step

Wrong level for the nested section.
