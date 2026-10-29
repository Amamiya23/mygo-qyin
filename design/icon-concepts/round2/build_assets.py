"""Render the final app PNGs from the editable SVG masters."""

from pathlib import Path
import sys

from PIL import Image, ImageFilter

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path.home() / ".codex/skills/logo-design/scripts"))
from render_png import render

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent


def desktop_icon(source: Path, output: Path) -> None:
    source_png = HERE / f".{source.stem}-transparent.png"
    if not render(source, str(source_png), 1024, 1024):
        raise RuntimeError(f"could not render {source}")

    artwork = Image.open(source_png).convert("RGBA")
    canvas = Image.new("RGBA", (1024, 1024))
    shadow_mask = artwork.getchannel("A").point(lambda a: round(a * 0.22))
    shadow = Image.new("RGBA", artwork.size, (0, 0, 0, 0))
    shadow.putalpha(shadow_mask)
    shadow = shadow.filter(ImageFilter.GaussianBlur(18))
    canvas.alpha_composite(shadow, (0, 10))
    canvas.alpha_composite(artwork)
    canvas.save(output, optimize=True)
    source_png.unlink()


def copy_png(source: Path, output: Path) -> None:
    Image.open(source).convert("RGBA").save(output, optimize=True)


def resize_png(source: Path, output: Path, size: int) -> None:
    image = Image.open(source).convert("RGBA")
    image.resize((size, size), Image.Resampling.LANCZOS).save(output, optimize=True)


if __name__ == "__main__":
    final = HERE / "b-turntable-final.svg"
    dev = HERE / "b-turntable-dev.svg"
    desktop_icon(final, ROOT / "resources/icon.png")
    desktop_icon(dev, ROOT / "resources/icon-dev.png")
    copy_png(ROOT / "resources/icon.png", ROOT / "website/public/icon.png")
    resize_png(ROOT / "resources/icon.png", ROOT / "internal/app/tray/tray.png", 32)
    copy_png(HERE / "a-flow-note-1024.png", HERE / "a-flow-note-alt.png")
    (ROOT / "website/public/favicon.svg").write_text(final.read_text(encoding="utf-8"), encoding="utf-8")
