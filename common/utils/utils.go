/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import "github.com/zhigui-projects/go-hotstuff/protos/pb"

func GetQuorumSize(metadata *pb.ConfigMetadata) int {
	return int(metadata.N - metadata.F)
}
