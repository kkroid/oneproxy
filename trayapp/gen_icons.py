"""Generate tray icons for OneProxy — rounded badge with network nodes."""
from PIL import Image, ImageDraw

SIZE = 256  # master size, downscale for ico

def rounded_rect(draw, box, radius, fill):
    draw.rounded_rectangle(box, radius=radius, fill=fill)

def make_icon(bg, node):
    """bg = badge color, node = node/line color."""
    img = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    # badge — fill entire canvas, no margin
    rounded_rect(d, (0, 0, SIZE, SIZE), radius=64, fill=bg + (255,))
    # network: 1 center hub + 3 satellites, connected
    cx, cy = SIZE // 2, SIZE // 2
    hub_r = 28
    sat_r = 20
    sats = [(cx, cy - 74), (cx - 64, cy + 46), (cx + 64, cy + 46)]
    # lines
    for sx, sy in sats:
        d.line((cx, cy, sx, sy), fill=node + (255,), width=12)
    # hub
    d.ellipse((cx - hub_r, cy - hub_r, cx + hub_r, cy + hub_r), fill=node + (255,))
    # satellites
    for sx, sy in sats:
        d.ellipse((sx - sat_r, sy - sat_r, sx + sat_r, sy + sat_r), fill=node + (255,))
    return img

def save_ico(img, path):
    sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    img.save(path, format="ICO", sizes=sizes)
    print(f"{path} saved")

WHITE = (255, 255, 255)
# green = all healthy, yellow = partial, red = down/stopped
save_ico(make_icon((0x2E, 0x8B, 0x57), WHITE), "green.ico")
save_ico(make_icon((0xE0, 0xA0, 0x00), WHITE), "yellow.ico")
save_ico(make_icon((0xC0, 0x39, 0x2B), WHITE), "red.ico")
print("done")
