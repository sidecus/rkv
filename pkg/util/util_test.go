package util

import "testing"

func TestMin(t *testing.T) {
	ret := Min(-1, 0)
	if ret != -1 {
		t.Error("Min deosn't return correct value when left operand is smaller")
	}

	ret = Min(0, -1)
	if ret != -1 {
		t.Error("Min deosn't return correct value when right operand is smaller")
	}
}

func TestMax(t *testing.T) {
	ret := Max(0, -1)
	if ret != 0 {
		t.Error("Max deosn't return correct value when left operand is bigger")
	}

	ret = Max(-1, 0)
	if ret != 0 {
		t.Error("Min deosn't return correct value when right operand is bigger")
	}
}
