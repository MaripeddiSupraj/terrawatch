# Security Policy

## Supported code

Security fixes are made against the latest code on `main` and the most recent public release when one exists.

## Reporting a vulnerability

Please do **not** open a public issue containing exploit details, credentials, Terraform state, cloud account identifiers, or sensitive plan output.

Use GitHub's **Security → Report a vulnerability** flow when it is available for this repository. If private vulnerability reporting is unavailable, open a minimal public issue asking for a private contact channel without including sensitive technical details.

Useful reports include:

- affected TerraWatch version or commit
- Terraform/OpenTofu version
- the smallest safe reproduction you can provide
- security impact and whether credentials/state/plan data may be exposed

TerraWatch executes Terraform/OpenTofu with the credentials available to the calling environment. Treat CI credentials, state backends, generated plan output, VCS tokens, and drift-report branches as security-sensitive.
