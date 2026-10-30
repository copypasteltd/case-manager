# Binary PDF publication classification

The first candidate publication check passed nineteen checks and rejected only
the source-whitespace list: four synthetic ReportLab PDF fixtures have required
internal whitespace and contain no NUL byte in the first 8 KiB. The temporary
publication-list preparer had therefore classified them as source text.

The failed [original result](repository-check.json) is retained without changing
its status. The corrected list treats the fixtures as binary PDF documents,
consistent with the repository's existing `*.pdf binary` attribute. All fixture
bytes, digests, native parsing results and functional inputs stay fixed. The
complete-file publication, private-value, credential-pattern and artifact checks
still include these files. Only source whitespace formatting excludes binaries.

The final current result is [recorded separately](../../repository-check.json).
