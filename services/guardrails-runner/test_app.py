import json
import unittest

from app import RequestError, validate


class GuardrailsRunnerTests(unittest.TestCase):
    def test_validates_a_fixed_action_proposal(self):
        result = validate(
            {
                "schema": "action_proposal",
                "proposal": json.dumps(
                    {
                        "title": "Review source evidence",
                        "summary": "Compare the available source identifiers before drafting a response.",
                        "risk": "medium",
                        "requiresApproval": True,
                        "nextAction": "Open the evidence review queue.",
                        "sourceRefs": ["source_1", "case-2"],
                    }
                ),
            }
        )
        self.assertEqual(result["status"], "valid")
        self.assertTrue(result["valid"])
        self.assertEqual(len(result["proposalDigest"]), 64)
        self.assertNotIn("Review source evidence", str(result))

    def test_rejects_invalid_or_nonopaque_proposals(self):
        invalid = validate(
            {
                "schema": "action_proposal",
                "proposal": '{"title":"","risk":"urgent","requiresApproval":false}',
            }
        )
        self.assertEqual(invalid["status"], "needs_review")
        with self.assertRaises(RequestError):
            validate(
                {
                    "schema": "action_proposal",
                    "proposal": json.dumps(
                        {
                            "title": "Review",
                            "summary": "A valid summary.",
                            "risk": "low",
                            "requiresApproval": False,
                            "nextAction": "Inspect.",
                            "sourceRefs": ["not a source link"],
                        }
                    ),
                }
            )


if __name__ == "__main__":
    unittest.main()
