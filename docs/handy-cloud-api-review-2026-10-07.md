# Handy Cloud API connection-key review — 2026-10-07

## Report and cause

A user reported "Handy connection key is malformed" for a five-character key.
Alpha.56's `BuildCloudAuthMetadata` rejected every key shorter than eight
characters. The app constructs this metadata before issuing its read-only
`GET /hsp/state` connection probe, so this error could occur without any
request reaching Handy's servers. No user key is needed to reproduce it.

## Primary references

- [Current API v3 OpenAPI specification](https://www.handyfeeling.com/api/handy-rest/v3/docs/spec.yaml), retrieved 2026-10-07:
  `components.schemas.ConnectionKey` specifies `minLength: 5`, `maxLength: 64`
  and `^[a-zA-Z0-9]{5,64}$`.
- [API v3 Swagger UI](https://www.handyfeeling.com/api/handy-rest/v3/docs/)
  loads that specification. Its `ConnectionKey` header parameter references
  the same schema, including for `GET /hsp/state`.
- [Ohdoki's Online Security and the Handy article](https://intercom.help/ohdoki/en/articles/9034256-online-security-and-the-handy)
  describes generated keys as 5–32 upper/lowercase letters and digits. That
  help text has a narrower maximum than the current API schema. Both
  references allow five characters; neither supports an eight-character
  minimum. The Cloud implementation follows the current API schema.

## Implementation review

REST requests use `X-Api-Key` for the Application ID and `X-Connection-Key`
for the device key. The SSE endpoint uses `apikey` and `ck` query parameters;
MagicHandy builds those with `url.Values` and sanitizes endpoint errors before
recording diagnostics. REST headers match the current specification and the
manufacturer's `@ohdoki/handy-sdk` 2.3.4 REST methods.

The SSE endpoint description explicitly documents query parameter `ck`, while
its shared parameter list also references the required `X-Connection-Key`
header. This reference inconsistency is separate from the local length bug.
The existing query-based SSE implementation follows the endpoint description;
this review does not claim live verification of a five-character SSE session.

Connection checks use `GET /hsp/state`, require a recognized positive HSP
response and do not send motion. A successful HTTP status alone is insufficient.
Application authentication, device availability and firmware/HSP support are
separate checks from a key's format. A syntactically valid key does not prove
that the device is online or that the key belongs to it.

The correction accepts 5–64 ASCII letters or digits, trims pasted outer
whitespace and preserves case. It rejects shorter, longer, non-alphanumeric
and internally whitespace-containing keys locally. The diagnostic states the
accepted format without echoing a key. Firmware v4/API v3/HSP prerequisites,
Stop handling and the shared motion path retain their existing gates.

## Regression coverage and review scope

The new boundary tests reproduce the old rejection of five-, six- and
seven-character keys and the old acceptance of overlong keys. They also check
32/64-character keys, pasted outer whitespace, case preservation, invalid
characters and non-disclosing fail-closed errors.

Both transport and HTTP API Cloud fixtures now use a synthetic five-character
key. Existing REST dispatch, SSE query-authentication, Stop, prerequisite,
diagnostic and trace-redaction tests therefore exercise the shortest allowed
key. The HTTP connection-check test additionally checks `GET /hsp/state` and
the unchanged key header. All Cloud responses in these tests come from local
mock servers; no real connection key or device is contacted.

The reported user's Cloud authentication and firmware were not tested. A user
on alpha.56 can generate an eight-character or longer key through Handy's
own configuration tools as a temporary workaround, then update the app's
saved key. The format correction requires a subsequent app update.
