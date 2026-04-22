---
name: oci-devops-scm-pr-cli
description: Use when asked to create, inspect, or troubleshoot Oracle OCI DevOps SCM pull requests from the command line, including repository OCID lookup, reviewer JSON preparation, and `oci devops pull-request` commands.
---

# OCI DevOps SCM PR CLI

## Overview

Use this skill to execute Oracle Cloud Infrastructure (OCI) DevOps pull-request workflows through `oci` CLI. Apply it when users ask for command-line PR creation in "DevOps SCM" and the environment is Oracle OCI.

## Command-Line Workflow

1. Confirm CLI auth and active profile:
```bash
oci iam region-subscription list
```

2. Find repository OCID from a project:
```bash
oci devops repository list --project-id <project_ocid> --all --output table
```

3. Prepare reviewers (optional, safest via generated schema):
```bash
oci devops pull-request create --generate-param-json-input reviewers > reviewers.json
```
Expected reviewer item shape:
```json
[
  { "principalId": "ocid1.user.oc1..exampleuniqueID" }
]
```

4. Create pull request:
```bash
PR_ID=$(
  oci devops pull-request create \
    --repository-id "<repo_ocid>" \
    --display-name "feat: short title" \
    --description "What changed and why" \
    --source-branch "feature/my-branch" \
    --destination-branch "main" \
    --reviewers file://reviewers.json \
    --query 'data.id' --raw-output
)
```
Notes:
- `--destination-branch` is optional; OCI uses the repository default branch when omitted.
- For fork-based PRs, add `--source-repository-id "<fork_repo_ocid>"`.

5. Inspect or list PRs:
```bash
oci devops pull-request get --pull-request-id "$PR_ID"
oci devops pull-request list-pull-requests \
  --repository-id "<repo_ocid>" \
  --lifecycle-details OPEN \
  --all --output table
```

## Output Conventions

- Prefer `--query` + `--raw-output` when user needs an ID for the next command.
- Prefer `--output table` for human-readable listing commands.
- When user asks for "just commands", return terse command blocks with placeholders.

## Common Failures

- `NotAuthorizedOrNotFound`: verify tenancy policy permissions and OCIDs.
- Empty list from repository query: wrong `project_ocid` or profile/region mismatch.
- Reviewer validation errors: use `--generate-param-json-input reviewers` and supply valid `principalId` OCIDs.
- Duplicate-PR behavior: OCI may reject same source/destination pair if an active PR already exists; list OPEN PRs first.
