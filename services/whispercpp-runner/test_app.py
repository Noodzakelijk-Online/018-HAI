import unittest

from app import selected_folder


class SelectedFolderTests(unittest.TestCase):
    def test_rejects_unbounded_paths(self):
        for value in ("", ".", "../outside", "/absolute"):
            with self.assertRaises(ValueError):
                selected_folder(value)


if __name__ == "__main__":
    unittest.main()
