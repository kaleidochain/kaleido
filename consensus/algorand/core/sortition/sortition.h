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

#ifdef __cplusplus
extern "C" {
#endif

  typedef unsigned long long ulonglong;

  ulonglong SortitionByStandardSearch(const void *hash, ulonglong n, ulonglong t, ulonglong W);
  ulonglong SortitionByBinarySearch(const void *hash, ulonglong n, ulonglong t, ulonglong W);

  // SortitionIsZeroByStandardSearch only checks if choosed weight is zero
  // resturns 1 if zero, 0 otherwise
  int SortitionIsZeroByStandardSearch(const void *hash, ulonglong n, ulonglong t, ulonglong W);

#ifdef __cplusplus
}
#endif

