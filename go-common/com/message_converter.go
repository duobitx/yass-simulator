package com

import "k8s.io/apimachinery/pkg/util/json"

func MsgMarshall(model any) ([]byte, error) {
	return json.Marshal(model)
}

func MsgUnmarshall(data []byte, model any) error {
	return json.Unmarshal(data, model)
}
