from evaluation.full_evaluation import allocate_same_budget, summarize
import unittest
class FullEvaluationTests(unittest.TestCase):
    def test_summary_is_deterministic_and_bounded(self):
        result=summarize([1,2,3,4,5])
        self.assertEqual(result["mean"],3);self.assertLess(result["ci95_lower"],3);self.assertGreater(result["ci95_upper"],3);self.assertEqual(result["minimum"],1)
    def test_budget_policies_share_every_constraint(self):
        candidates=[{"case_id":"a","sequence":0,"expected_nerv_minor":10,"expected_incremental_value_minor":11,"expected_cost_minor":10,"is_contact":True,"is_retry":False},{"case_id":"b","sequence":1,"expected_nerv_minor":100,"expected_incremental_value_minor":101,"expected_cost_minor":1,"is_contact":False,"is_retry":True}]
        fcfs=allocate_same_budget(candidates,False);greedy=allocate_same_budget(candidates,True)
        self.assertEqual(fcfs["budget"],greedy["budget"])
        for result in (fcfs,greedy):
            for resource,limit in result["budget"].items(): self.assertLessEqual(result["used"][resource],limit)
