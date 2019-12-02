// Copyright (c) 2019 The kaleido Authors
// This file is part of kaleido
//
// kaleido is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// kaleido is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with kaleido. If not, see <https://www.gnu.org/licenses/>.

#include "sortition.h"

#include <stdlib.h>
#include <assert.h>
#include <boost/math/distributions/binomial.hpp>
#include <boost/multiprecision/cpp_bin_float.hpp>

using boost::math::binomial_distribution;
using boost::math::cdf;

using namespace boost::multiprecision;
typedef number<cpp_bin_float<256, digit_base_2> > float256_t;
using boost::multiprecision::uint256_t;

typedef double float52_t;

typedef float52_t floatx_t;
typedef binomial_distribution<floatx_t> dist_t;

// ==================

const floatx_t float_one(1);

static inline
floatx_t hash2float(const void *hash) {
  const unsigned char * const p_begin = (const unsigned char *)hash;
  const unsigned char * const p_end = p_begin + 256 / 8;

  uint256_t hashInt;
  import_bits(hashInt, p_begin, p_end);
  float256_t hashFloat(hashInt);
  float256_t max(std::numeric_limits<uint256_t>::max());
  float256_t tmp(hashFloat / max);
  return tmp.convert_to<floatx_t>();
}

const ulonglong delta = 0;
static inline
ulonglong MaxJ(ulonglong n, ulonglong t) {
  return std::min(n, t + delta);
}

// ================

ulonglong SortitionByStandardSearch(const void *hash, ulonglong n, ulonglong t, ulonglong W) {
  assert(n <= W && t <= W);
  floatx_t h = hash2float(hash);
  dist_t dist(n,  floatx_t(t) / floatx_t(W));

  register ulonglong maxj = MaxJ(n, t);
  register ulonglong j = 0;
  while(cdf(dist, j) <= h) {  // h NOT between in [0, F(j))
    j++;
    if (j >= maxj) {
      break;
    }
  }

  return j;
}

// ===================

ulonglong SortitionByBinarySearch(const void *hash, ulonglong n, ulonglong t, ulonglong W) {
  assert(n <= W && t <= W);
  floatx_t h = hash2float(hash);
  dist_t dist(n,  floatx_t(t) / floatx_t(W));

  // fast check for j=0
  if (h < cdf(dist, 0)) {       // h between in [0, F(0))
    return 0;
  }

  register ulonglong low = 1;
  register ulonglong high = MaxJ(n, t);

  while (low < high) {
    register ulonglong middle = low + (high - low) / 2;

    if (cdf(dist, middle) <= h) { // h NOT between in [0, F(middle))
      low = middle + 1;           // j must in [middle+1, high]
    } else {                      // h between in [0, F(middle))
      high = middle;              // j must in [low, middle]
    }
  }

  return low;
}

// =================

int SortitionIsZeroByStandardSearch(const void *hash, ulonglong n, ulonglong t, ulonglong W) {
  assert(n <= W && t <= W);
  floatx_t h = hash2float(hash);
  dist_t dist(n,  floatx_t(t) / floatx_t(W));

  if (h < cdf(dist, 0)) {       // h between in [0, F(0))
    return 1;
  }
  return 0;
}
