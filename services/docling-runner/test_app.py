import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

import app


class SelectedFolderTests(unittest.TestCase):
    def test_rejects_unbounded_paths_before_reading_any_document(self):
        for value in ("", ".", "../outside", "/absolute"):
            with self.assertRaises(ValueError):
                app.selected_folder(value)

    def test_candidate_files_are_bounded_to_allowed_documents(self):
        with TemporaryDirectory() as directory:
            folder = Path(directory)
            for index in range(app.MAX_DOCUMENTS + 3):
                (folder / f"brief-{index}.md").write_text("reviewable text", encoding="utf-8")

            candidates = app.candidate_files(folder, pdf_enabled=False)

            self.assertEqual(app.MAX_DOCUMENTS, len(candidates))
            self.assertTrue(all(file_format == "markdown" for _, file_format in candidates))


if __name__ == "__main__":
    unittest.main()
