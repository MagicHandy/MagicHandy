# Release Checks And Update Handoff

MagicHandy can discover a newer stable release without silently downloading or
executing one. The core performs the network request, the embedded UI reports
the result, and the user remains in control of the actual update.

## Current Behavior

- Versioned builds check GitHub Releases after the core and first-run setup are
  ready. Source `dev` builds do not make an automatic request because their Git
  branch, not a release tag, determines whether they are current.
- **Settings > General > Updates** can switch automatic checks to manual-only
  and can run an explicit check at any time.
- A newer stable semantic version creates one deduplicated notification for that
  release in the existing notification center. Selecting it opens General
  settings, where **View release** opens the canonical project release page.
- No release, an up-to-date build, a development build, and a failed check are
  distinct results. Automatic network failures do not raise a startup alarm;
  an explicit check reports the failure in place.

GitHub's `GET /repos/MagicHandy/MagicHandy/releases/latest` endpoint is the
source of truth. It selects the latest published stable release rather than a
draft or prerelease. The request is unauthenticated, sends the recommended
GitHub API headers, caches a successful result for six hours, and revalidates
with `ETag` / `If-None-Match`. GitHub documents an unauthenticated limit of 60
requests per hour per source IP; the cache keeps normal app use well below it.
An unavailable request is retried automatically no more than once every 15
minutes. **Check now** remains an explicit bypass, and a previous successful
result stays visible as stale rather than disappearing during an outage.

References:

- [GitHub REST release endpoints](https://docs.github.com/en/rest/releases/releases)
- [GitHub REST API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)

## Update Paths

Packaged users review the release notes and checksums, download the newer setup
EXE, and run it over the existing installation. Inno Setup uses the stable app
identity, closes the running `magichandy.exe`, replaces program files, and
leaves `%APPDATA%\MagicHandy` intact. Database migrations remain the new core's
responsibility at startup.

Portable users replace the extracted application directory with the new
portable payload while the app is stopped. Their data also remains under the
normal app-data directory unless they deliberately use `-data-dir`.

Source users continue to run:

```powershell
.\update.ps1
```

That script fast-forwards a clean checkout and rebuilds the core without
replaying optional runtime choices. A GitHub release notification is not a
substitute for checking the source branch.

## Trust Boundary

The release checker deliberately does not:

- accept a repository or download URL from the browser;
- send a GitHub token, settings, device credentials, or diagnostics;
- read or display arbitrary release HTML;
- download, verify, launch, or schedule an installer;
- stop motion or take controller ownership; or
- treat update discovery as required for startup.

The local API returns a canonical `github.com/MagicHandy/MagicHandy` release
URL constructed from the release tag. Only the latest stable semantic version
is comparable. The setting is shared through the backend settings snapshot, so
multiple tabs do not maintain conflicting update preferences.

## Local API

`GET /api/update` returns the cached result when it is fresh.
`GET /api/update?refresh=1` performs a conditional refresh for the explicit
**Check now** action. States are `available`, `current`, `development`,
`no_release`, and `error`. A failed refresh may return the last successful
result with `stale: true`.

## Future Automatic Installer

Downloading or applying an update inside MagicHandy remains deferred. It needs
a signed release manifest, production code signing, checksum/signature
verification before execution, mandatory motion shutdown, database backup and
compatibility policy, rollback, interrupted-update recovery, and acceptance
tests for active controller sessions. Release discovery does not weaken those
requirements.
