# Simulator

`decision-service/simulation` separates observable customer/payment/merchant history from hidden response characteristics. A stable seed and hashes determine populations and potential outcomes independent of iteration order. Train, validation, and test JSONL files are generated with a distribution report. Optimizers receive only `observable`; hidden fields exist solely for outcome generation and evaluation.
