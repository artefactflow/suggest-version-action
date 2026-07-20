# Security policy

## Reporting a vulnerability

Please do not open a public issue for a security problem.

Report it through GitHub's private vulnerability reporting: open the **Security** tab of this repository and choose **Report a vulnerability**. That opens a private channel visible only to maintainers.

Include what you did, what happened, and what you expected — a minimal reproduction is worth more than anything else. You will get an acknowledgement, and we will keep you updated as the report is triaged and fixed. Please give us a chance to ship a fix before disclosing publicly.

## Supported versions

Fixes go onto the newest major release, and the moving major tag (for example `v1`) is repointed at it. Pin to the major tag to pick fixes up automatically, or to a full release tag if you would rather review each change.

## Handling of credentials

This action takes a bearer token as an input. It is used for exactly one thing — authenticating the API call — and it is never written to a log line, an error message, or an action output. Pass it from a secret rather than inlining it, so the runner masks it in the log if anything ever does print it.
