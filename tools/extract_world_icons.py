"""按 DST 图集 XML 的 UV 坐标切出 PNG；需要 Pillow，不执行模组代码。"""
import argparse
from pathlib import Path
import re
import xml.etree.ElementTree as ET

from PIL import Image


def regions(xml_path, size):
    width, height = size
    result = {}
    for element in ET.parse(xml_path).findall('./Elements/Element'):
        name = element.attrib['name']
        if not re.fullmatch(r'[a-zA-Z0-9_]+\.tex', name):
            raise ValueError(f'非法图标名：{name}')
        name = Path(name).with_suffix('.png').name
        if name in result:
            raise ValueError(f'图标名重复：{name}')
        u1, u2, v1, v2 = (float(element.attrib[key]) for key in ('u1', 'u2', 'v1', 'v2'))
        if not (0 <= u1 < u2 <= 1 and 0 <= v1 < v2 <= 1):
            raise ValueError(f'UV 坐标越界：{name}')
        # DST 的 UV 指向像素中心，且 V 从底部起算；PNG 从左上角起算。
        box = (
            round(u1 * width - 0.5), round((1 - v2) * height - 0.5),
            round(u2 * width + 0.5), round((1 - v1) * height + 0.5),
        )
        left, top, right, bottom = box
        if not (0 <= left < right <= width and 0 <= top < bottom <= height):
            raise ValueError(f'裁切区域越界或为空：{name} {box}')
        result[name] = box
    if not result:
        raise ValueError('XML 中没有图标定义')
    return result


def extract(image_path, xml_path, output, check=False):
    with Image.open(image_path) as source:
        atlas = source.convert('RGBA')
    boxes = regions(xml_path, atlas.size)
    if not check:
        output.mkdir(parents=True, exist_ok=True)
    for name, box in boxes.items():
        crop = atlas.crop(box)
        target = output / name
        if check:
            with Image.open(target) as saved:
                actual = saved.convert('RGBA')
                if actual.size != crop.size or actual.tobytes() != crop.tobytes():
                    raise ValueError(f'切片与原图不一致：{target}')
        else:
            crop.save(target)
    return len(boxes)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('image', type=Path, help='解码后的 PNG 图集')
    parser.add_argument('xml', type=Path, help='对应的图集 XML')
    parser.add_argument('output', type=Path, help='单图标输出目录')
    parser.add_argument('--check', action='store_true', help='只校验已有切片，不写文件')
    args = parser.parse_args()
    try:
        count = extract(args.image, args.xml, args.output, args.check)
    except (OSError, ValueError, KeyError, ET.ParseError) as error:
        parser.exit(1, f'图标提取失败：{error}\n')
    print(f'{"校验通过" if args.check else "已切出"} {count} 个图标：{args.output}')
