# Publication documentation corrections

The first candidate publication scan passed the functional, sensitive-file and local-link checks, but rejected two packaging details: both new language-guide introductions lacked the current shared-mobile test count (465), and the new stopped-build raw log contained its original trailing whitespace.

The guides receive the actual current check counts. The exact stopped-build log is preserved losslessly as gzip with original and archive SHA-256 metadata, rather than changing diagnostic bytes. These are documentation/evidence packaging corrections only; all nine executed application/build checks keep the same frozen sources. Scope and publication consistency checks are executed again. This failed candidate is not relabelled as passed.
