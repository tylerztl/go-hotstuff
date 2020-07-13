/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

type Database interface {
	Get(key interface{}) (interface{}, error)
	Put(key interface{}, value interface{}) error
	Delete(key interface{}) error
	Close()
}
