package goby

type Integer struct {
	num int
}

func NewInteger(num int) Integer {
	return Integer{num: num}
}
