package main

// trackerQualityBaseline is the committed upper bound on tracker SID true warning count.
// It must not increase. To lower it: fix the underlying issue, verify the new count,
// then update this constant and commit.
const trackerQualityBaseline = 581

