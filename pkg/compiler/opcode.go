package compiler

type Opcode byte

type Instructions []byte

const (
	OpConstant Opcode = iota

	OpPop

	OpTrue
	OpFalse
	OpNil

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
	OpMinus
	OpBang

	OpEqual
	OpNotEqual
	OpGreaterThan
	OpGreaterThanOrEqual
	OpLessThan
	OpLessThanOrEqual

	OpJump
	OpJumpNotTruthy
	OpJumpNotNil
	OpJumpTruthy

	OpArray
	OpHash

	OpIndex
	OpIndexAssign

	OpGetGlobal
	OpSetGlobal

	OpGetLocal
	OpSetLocal

	OpGetFree
	OpSetFree

	OpGetOuter
	OpSetOuter

	OpGetInstanceVar
	OpSetInstanceVar

	OpGetClassVar
	OpSetClassVar

	OpGetConstant
	OpSetConstant

	OpClosure

	OpCurrentClosure

	OpReturn
	OpReturnValue

	OpSend
	OpSendWithBlock
	OpSendSuper

	OpDefineMethod
	OpDefineClassMethod
	OpDefineFunction

	OpClass
	OpModule

	OpInherited
	OpIncluded
	OpExtended

	OpOpenClass
	OpOpenClassWithSuper

	OpLambda
	OpBlock
	OpBlockWithArg

	OpBreak
	OpBreakValue

	OpMatch
	OpNotMatch

	OpToAry

	OpDup

	OpBitAnd
	OpBitOr
	OpBitXor
	OpBitNot
	OpBitLeftShift
	OpBitRightShift

	OpNegate

	OpSelf

	OpNeg

	OpYield
	OpYieldWithValue

	OpRescue
	OpRescueMatch

	OpRetry
	OpRaise

	OpExtend
	OpPrepend

	OpAlias
	OpUndef

	OpDefined

	OpCaseEq

	OpIsA
	OpKindOf

	OpInstanceOf
	OpRespondTo

	OpClassOf

	OpFreeze

	OpSplat

	OpDebug
)

type Definition struct {
	Name          string
	OperandWidths []int
}

var definitions = map[Opcode]Definition{
	OpConstant: {"OpConstant", []int{2}},
	OpPop:      {"OpPop", []int{}},
	OpTrue:     {"OpTrue", []int{}},
	OpFalse:    {"OpFalse", []int{}},
	OpNil:      {"OpNil", []int{}},

	OpAdd:   {"OpAdd", []int{}},
	OpSub:   {"OpSub", []int{}},
	OpMul:   {"OpMul", []int{}},
	OpDiv:   {"OpDiv", []int{}},
	OpMod:   {"OpMod", []int{}},
	OpPow:   {"OpPow", []int{}},
	OpMinus: {"OpMinus", []int{}},
	OpBang:  {"OpBang", []int{}},

	OpEqual:              {"OpEqual", []int{}},
	OpNotEqual:           {"OpNotEqual", []int{}},
	OpGreaterThan:        {"OpGreaterThan", []int{}},
	OpGreaterThanOrEqual: {"OpGreaterThanOrEqual", []int{}},
	OpLessThan:           {"OpLessThan", []int{}},
	OpLessThanOrEqual:    {"OpLessThanOrEqual", []int{}},

	OpJump:          {"OpJump", []int{2}},
	OpJumpNotTruthy: {"OpJumpNotTruthy", []int{2}},
	OpJumpNotNil:    {"OpJumpNotNil", []int{2}},
	OpJumpTruthy:    {"OpJumpTruthy", []int{2}},

	OpArray: {"OpArray", []int{2}},
	OpHash:  {"OpHash", []int{2}},

	OpIndex:       {"OpIndex", []int{}},
	OpIndexAssign: {"OpIndexAssign", []int{}},

	OpGetGlobal: {"OpGetGlobal", []int{2}},
	OpSetGlobal: {"OpSetGlobal", []int{2}},

	OpGetLocal: {"OpGetLocal", []int{1}},
	OpSetLocal: {"OpSetLocal", []int{1}},

	OpGetFree: {"OpGetFree", []int{1}},
	OpSetFree: {"OpSetFree", []int{1}},

	OpGetOuter: {"OpGetOuter", []int{1}},
	OpSetOuter: {"OpSetOuter", []int{1, 1}},

	OpGetInstanceVar: {"OpGetInstanceVar", []int{2}},
	OpSetInstanceVar: {"OpSetInstanceVar", []int{2}},

	OpGetClassVar: {"OpGetClassVar", []int{2}},
	OpSetClassVar: {"OpSetClassVar", []int{2}},

	OpGetConstant: {"OpGetConstant", []int{2}},
	OpSetConstant: {"OpSetConstant", []int{2}},

	OpClosure:        {"OpClosure", []int{2, 1}},
	OpCurrentClosure: {"OpCurrentClosure", []int{}},

	OpReturn:      {"OpReturn", []int{}},
	OpReturnValue: {"OpReturnValue", []int{}},

	OpSend:          {"OpSend", []int{2, 1, 1}},
	OpSendWithBlock: {"OpSendWithBlock", []int{2, 1, 1, 2}},
	OpSendSuper:     {"OpSendSuper", []int{2, 1, 1}},

	OpDefineMethod:      {"OpDefineMethod", []int{2}},
	OpDefineClassMethod: {"OpDefineClassMethod", []int{2}},
	OpDefineFunction:    {"OpDefineFunction", []int{2}},

	OpClass:  {"OpClass", []int{2}},
	OpModule: {"OpModule", []int{2}},

	OpInherited: {"OpInherited", []int{}},
	OpIncluded:  {"OpIncluded", []int{}},
	OpExtended:  {"OpExtended", []int{}},

	OpOpenClass:          {"OpOpenClass", []int{2}},
	OpOpenClassWithSuper: {"OpOpenClassWithSuper", []int{2}},

	OpLambda:       {"OpLambda", []int{2}},
	OpBlock:        {"OpBlock", []int{2}},
	OpBlockWithArg: {"OpBlockWithArg", []int{2, 1}},

	OpBreak:      {"OpBreak", []int{}},
	OpBreakValue: {"OpBreakValue", []int{}},

	OpMatch:    {"OpMatch", []int{}},
	OpNotMatch: {"OpNotMatch", []int{}},

	OpToAry: {"OpToAry", []int{}},
	OpDup:   {"OpDup", []int{}},

	OpBitAnd:        {"OpBitAnd", []int{}},
	OpBitOr:         {"OpBitOr", []int{}},
	OpBitXor:        {"OpBitXor", []int{}},
	OpBitNot:        {"OpBitNot", []int{}},
	OpBitLeftShift:  {"OpBitLeftShift", []int{}},
	OpBitRightShift: {"OpBitRightShift", []int{}},

	OpNegate: {"OpNegate", []int{}},
	OpSelf:   {"OpSelf", []int{}},
	OpNeg:    {"OpNeg", []int{}},

	OpYield:          {"OpYield", []int{}},
	OpYieldWithValue: {"OpYieldWithValue", []int{1}},

	OpRescue:      {"OpRescue", []int{2}},
	OpRescueMatch: {"OpRescueMatch", []int{}},
	OpRetry:       {"OpRetry", []int{}},
	OpRaise:       {"OpRaise", []int{}},

	OpExtend:  {"OpExtend", []int{}},
	OpPrepend: {"OpPrepend", []int{}},

	OpAlias: {"OpAlias", []int{}},
	OpUndef: {"OpUndef", []int{}},

	OpDefined:    {"OpDefined", []int{2}},
	OpCaseEq:     {"OpCaseEq", []int{}},
	OpIsA:        {"OpIsA", []int{}},
	OpKindOf:     {"OpKindOf", []int{}},
	OpInstanceOf: {"OpInstanceOf", []int{}},
	OpRespondTo:  {"OpRespondTo", []int{}},
	OpClassOf:    {"OpClassOf", []int{}},
	OpFreeze:     {"OpFreeze", []int{}},
	OpSplat:      {"OpSplat", []int{}},
	OpDebug:      {"OpDebug", []int{}},
}

func Lookup(op byte) (Definition, bool) {
	def, ok := definitions[Opcode(op)]
	return def, ok
}
