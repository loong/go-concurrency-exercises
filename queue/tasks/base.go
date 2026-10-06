package tasks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

const (
	SumTaskType     = "sum"
	HashTaskType    = "hash"
	BurnCPUTaskType = "BurnCPUTask"
)

type Task struct {
	Id    string `json:"id"`
	Type  string `json:"type"`
	Input []byte `json:"input"`
}

type SumTaskInput struct {
	A int `json:"a"`
	B int `json:"b"`
}

type SumTaskOutput struct {
	Res int `json:"res"`
}

func SumTask(input []byte) ([]byte, error) {
	inputData := SumTaskInput{}
	err := json.Unmarshal(input, &inputData)
	if err != nil {
		fmt.Println("Unmarshal error:", err)
		return nil, err
	}

	res := SumTaskOutput{Res: inputData.A + inputData.B}
	return json.Marshal(res)
}

type HashTaskInput struct {
	Iteration int `json:"iteration"`
}

type HashTaskOutput struct {
	Res string `json:"res"`
}

func HashTask(iterations int) []byte {
	data := []byte("benchmark")
	var sum [32]byte

	for i := 0; i < iterations; i++ {
		sum = sha256.Sum256(data)
	}

	return sum[:]
}

type BurnCPUTaskInput struct {
	Iteration int `json:"iteration"`
}

type BurnCPUTaskOutput struct {
	Res int `json:"res"`
}

func BurnCPUTask(input []byte) ([]byte, error) {
	inputType := BurnCPUTaskInput{}
	if err := json.Unmarshal(input, &inputType); err != nil {
		return nil, err
	}

	var x uint64 = 1
	for i := 0; i < inputType.Iteration; i++ {
		x = x*1664525 + 1013904223 // LCG, prevents optimization
	}

	bytes, _ := json.Marshal(inputType)
	return bytes, nil
}
