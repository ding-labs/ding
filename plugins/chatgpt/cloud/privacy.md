# Ding Cloud integration data handling

This is a technical disclosure. The listing links to the operator's published
privacy policy and terms, which must be supplied when building the package.

Ding Cloud stores account/workspace membership, watch definitions, observations,
alert evidence, delivery history, grants and mutation receipts. Requested tool
results are sent to ChatGPT and processed under that service's terms. Watch
sources and notification destinations receive the requests users configure.

OAuth and notification credentials belong in protected storage. Tool results may
contain secret reference names and status, but not their stored values. Selected
source data may be sensitive; choose monitored fields accordingly.

Revoking a connection blocks subsequent access. It does not erase information
already returned to ChatGPT or pause existing watches. Pause/delete watches
separately when desired. A notification retry can duplicate a delivery.
