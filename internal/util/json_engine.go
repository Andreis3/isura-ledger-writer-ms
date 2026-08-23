package util

import "github.com/bytedance/sonic"

// Frozen global instance, thread-safe and optimized for high performance
var JsonEngine = sonic.ConfigFastest
