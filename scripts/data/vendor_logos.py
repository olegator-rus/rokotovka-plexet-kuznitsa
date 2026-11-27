"""Значки производителей из папки исходных картинок (manifest.csv и файлы) во фронт.

Берёт manifest.csv (id, name, file), каждую картинку обрезает по полям (прозрачные и почти белые), вписывает
в квадрат 128×128 с прозрачным фоном и пишет frontend/public/vendors/<id>.png. Соответствие «имя производителя
в фиде → id» — в frontend/src/lib/vendorLogoFeed.ts (генерируется, руками не править).

    python3 -I scripts/data/vendor_logos.py <папка с manifest.csv>
"""
import csv
import json
import pathlib
import sys

from PIL import Image, ImageChops

ROOT = pathlib.Path(__file__).resolve().parents[2]
SRC = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "data/vendor-logos"
OUT = ROOT / "frontend/public/vendors"
TS = ROOT / "frontend/src/lib/vendorLogoFeed.ts"
SIZE, PAD = 128, 6


def trim(im: Image.Image) -> Image.Image:
    """Обрезка по содержимому: фон — прозрачность или почти белый цвет."""
    rgba = im.convert("RGBA")
    alpha = rgba.getchannel("A").point(lambda a: 255 if a > 16 else 0)
    white = Image.new("RGB", rgba.size, (255, 255, 255))
    diff = ImageChops.difference(rgba.convert("RGB"), white).convert("L").point(lambda d: 255 if d > 24 else 0)
    box = ImageChops.multiply(alpha, diff).getbbox()
    return rgba.crop(box) if box else rgba


def square(im: Image.Image) -> Image.Image:
    inner = SIZE - 2 * PAD
    im = im.copy()
    im.thumbnail((inner, inner), Image.LANCZOS)
    if max(im.size) < inner:  # маленькие исходники — до размера плитки
        k = inner / max(im.size)
        im = im.resize((max(1, round(im.width * k)), max(1, round(im.height * k))), Image.LANCZOS)
    canvas = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    canvas.paste(im, ((SIZE - im.width) // 2, (SIZE - im.height) // 2), im)
    return canvas


def main() -> int:
    rows = [r for r in csv.DictReader(open(SRC / "manifest.csv", encoding="utf-8")) if r.get("file")]
    names = {}
    for r in rows:
        dst = OUT / f"{r['id']}.png"
        square(trim(Image.open(SRC / r["file"]))).save(dst, optimize=True)
        names[r["name"].strip()] = r["id"]
    lines = ",\n".join(f"  {json.dumps(n, ensure_ascii=False)}: {json.dumps(i)}" for n, i in sorted(names.items()))
    TS.write_text(
        "// Генерирует scripts/data/vendor_logos.py из manifest.csv исходных значков — руками не править.\n"
        "// Имя производителя как в фиде заказчика → файл public/vendors/<id>.png (store.softline.ru, LICENSES.md).\n"
        f"export const FEED_LOGO_IDS: Record<string, string> = {{\n{lines},\n}}\n",
        encoding="utf-8",
    )
    print(f"значков: {len(rows)}, словарь: {len(names)} → {TS.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
