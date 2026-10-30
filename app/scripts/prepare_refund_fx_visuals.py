"""Decode current actual media for a separate human visual review."""
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
# Reuse the image layout only. All media inputs and output paths are scoped
# to this increment; no prior evidence or media file is opened for writing.
source = (APP / 'scripts/prepare_payment_fee_visuals.py').read_text(encoding='utf-8')
source = source.replace("'docs/quality/evidence/payment-fee'", "'docs/quality/evidence/refund-fx'")
source = source.replace("'app/.local/repository-showcase/payment-fee-visual-review'", "'app/.local/repository-showcase/refund-fx-visual-review'")
source = source.replace("E / 'media.json'", "E / 'media-encoding.json'")
source = source.replace("('payment-fee-' + client)", "('refund-fx-' + client)")
namespace = {'__name__': 'refund_fx_visuals', '__file__': str(Path(__file__))}
exec(compile(source, str(Path(__file__)), 'exec'), namespace)
if __name__ == '__main__':
    namespace['main']()
