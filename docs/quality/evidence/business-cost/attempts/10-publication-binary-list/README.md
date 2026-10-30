# Publication text-list correction

The first candidate publication scan found no credentials, forbidden paths or broken links, and all functional/source checks passed. Its sole failure was the text-whitespace check: the temporary publication-list preparation had incorrectly added15 PNG/GIF/MP4 files to a list intended for code and guide text. Arbitrary binary bytes naturally include space/tab sequences.

The preparation now excludes binary files from that text-only list using the same NUL-byte classification as the repository scanner. Actual media remains included in the full publication scan, source-bound media manifests, per-file hashes and visual review. No media byte, functional result, security check, migration exception or verifier is changed. The original failed candidate report is retained here; the corrected candidate and staged scans are executed separately.
