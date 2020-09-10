/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pb

import "github.com/golang/protobuf/proto"

func (x *HardState) Marshal() (data []byte, err error) {
	return proto.Marshal(x)
}

func (x *HardState) Unmarshal(data []byte) error {
	return proto.Unmarshal(data, x)
}
