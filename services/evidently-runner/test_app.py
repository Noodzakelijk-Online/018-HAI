import unittest

from app import RequestError, evaluate


class EvidentlyRunnerTests(unittest.TestCase):
    def test_runs_a_bounded_synthetic_fixture(self):
        result = evaluate({
            "fixtureKind": "synthetic",
            "cases": [
                {"id": "case_a", "input": "test question", "output": "test answer"},
                {"id": "case_b", "input": "another question", "output": "another answer"},
            ],
        })
        self.assertEqual(result["status"], "passed")
        self.assertEqual(result["caseCount"], 2)
        self.assertEqual(len(result["reportDigest"]), 64)
        self.assertNotIn("test answer", str(result))

    def test_rejects_invalid_or_oversized_cases(self):
        with self.assertRaises(RequestError):
            evaluate({"fixtureKind": "production", "cases": []})
        with self.assertRaises(RequestError):
            evaluate({"fixtureKind": "synthetic", "cases": [{"id": "email@example.test", "input": "a", "output": "b"}]})


if __name__ == "__main__":
    unittest.main()
