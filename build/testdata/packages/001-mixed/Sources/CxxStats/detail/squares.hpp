#pragma once
#include <numeric>
#include <vector>

namespace stats {
inline int sumSquares(int n) {
  std::vector<int> v;
  for (int i = 1; i <= n; i++)
    v.push_back(i * i);
  return std::accumulate(v.begin(), v.end(), 0) + STATS_BIAS - 1;
}
} // namespace stats
