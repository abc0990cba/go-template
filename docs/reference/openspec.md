# OpenSpec

OpenSpec is the required workflow for feature development in this repository.

Each change uses this layout:

```text
openspec/changes/<change-name>/
  proposal.md
  specs/<capability>/spec.md
  design.md
  tasks.md
```

## Rule

No feature is complete until:

- the change exists;
- behavior is described in specs;
- implementation tasks are checked off;
- tests are linked in the matrices;
- docs are updated.

## Validate

```bash
make openspec-validate
```

If the OpenSpec CLI is not installed, the command prints a clear message and exits successfully.
