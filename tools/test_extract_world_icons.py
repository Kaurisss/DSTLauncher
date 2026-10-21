import tempfile
from pathlib import Path
import unittest

from PIL import Image

from extract_world_icons import extract, regions


class ExtractWorldIconsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.xml = self.root / 'atlas.xml'
        self.xml.write_text('''<Atlas><Elements>
            <Element name="top_left.tex" u1="0.125" u2="0.375" v1="0.625" v2="0.875"/>
            <Element name="bottom_right.tex" u1="0.625" u2="0.875" v1="0.125" v2="0.375"/>
        </Elements></Atlas>''', encoding='utf-8')

    def test_pixel_centres_vertical_origin_and_alpha(self):
        self.assertEqual(regions(self.xml, (4, 4)), {
            'top_left.png': (0, 0, 2, 2), 'bottom_right.png': (2, 2, 4, 4),
        })
        source = Image.new('RGBA', (4, 4), (0, 0, 255, 255))
        source.paste((255, 0, 0, 255), (0, 0, 2, 2))
        source.putpixel((0, 0), (10, 20, 30, 0))
        png = self.root / 'atlas.png'
        source.save(png)
        output = self.root / 'icons'
        self.assertEqual(extract(png, self.xml, output), 2)
        with Image.open(output / 'top_left.png') as crop:
            self.assertEqual(crop.size, (2, 2))
            self.assertEqual(crop.getpixel((0, 0)), (10, 20, 30, 0))
            self.assertEqual(crop.getpixel((1, 1)), (255, 0, 0, 255))
        with Image.open(output / 'bottom_right.png') as crop:
            self.assertEqual(crop.getpixel((0, 0)), (0, 0, 255, 255))
        self.assertEqual(extract(png, self.xml, output, check=True), 2)
        Image.new('RGBA', (2, 2)).save(output / 'top_left.png')
        with self.assertRaisesRegex(ValueError, '切片与原图不一致'):
            extract(png, self.xml, output, check=True)

    def test_reject_invalid_regions_and_names(self):
        valid = '<Element name="ok.tex" u1="0.125" u2="0.375" v1="0.625" v2="0.875"/>'
        for body in [valid.replace('ok.tex', '../outside.tex'), valid * 2,
                     valid.replace('0.125', '-1'), valid.replace('0.375', 'nan'), '']:
            with self.subTest(body=body):
                self.xml.write_text(f'<Atlas><Elements>{body}</Elements></Atlas>', encoding='utf-8')
                with self.assertRaises(ValueError):
                    regions(self.xml, (4, 4))


if __name__ == '__main__':
    unittest.main()
