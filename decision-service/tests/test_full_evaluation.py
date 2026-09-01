from evaluation.full_evaluation import summarize
import unittest
class FullEvaluationTests(unittest.TestCase):
    def test_summary_is_deterministic_and_bounded(self):
        result=summarize([1,2,3,4,5])
        self.assertEqual(result["mean"],3);self.assertLess(result["ci95_lower"],3);self.assertGreater(result["ci95_upper"],3);self.assertEqual(result["minimum"],1)
