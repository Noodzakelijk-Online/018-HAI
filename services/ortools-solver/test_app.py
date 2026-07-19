import unittest

from app import RequestError, solve


class SolverTest(unittest.TestCase):
    def test_prioritizes_work_when_capacity_is_limited(self):
        result = solve({
            "dayStartMinute": 540,
            "dayEndMinute": 660,
            "jobs": [
                {"id": "high", "durationMinutes": 60, "priority": 90},
                {"id": "medium", "durationMinutes": 60, "priority": 50},
                {"id": "low", "durationMinutes": 60, "priority": 10},
            ],
        })
        self.assertIn(result["status"], ("optimal", "feasible"))
        self.assertEqual([item["id"] for item in result["scheduled"]], ["high", "medium"])
        self.assertEqual(result["deferred"], ["low"])

    def test_rejects_unbounded_or_invalid_input(self):
        with self.assertRaises(RequestError):
            solve({"jobs": [{"id": "x", "durationMinutes": 0}]})
        with self.assertRaises(RequestError):
            solve({"jobs": [{"id": "external url", "durationMinutes": 60}]})


if __name__ == "__main__":
    unittest.main()
