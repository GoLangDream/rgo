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
	OpFlipFlopGet
	OpFlipFlopSet

	OpArray
	OpArrayAppend
	OpHash
	OpHashMerge
	OpMarkKeywordHash

	OpIndex
	OpSliceIndex
	OpIndexAssign
	OpIndexCompoundAssign
	OpIndexSplatCompoundAssign

	OpGetGlobal
	OpSetGlobal

	OpGetLocal
	OpSetLocal
	OpGetLocalCell

	OpGetFree
	OpSetFree
	OpGetFreeCell

	OpGetOuter
	OpSetOuter
	OpGetOuterFree
	OpSetOuterFree
	OpGetOuterCell
	OpGetOuterFreeCell

	OpGetInstanceVar
	OpSetInstanceVar
	OpDefinedInstanceVar

	OpGetClassVar
	OpGetClassVarOrNil
	OpSetClassVar
	OpDefinedClassVar
	OpDefinedGlobal

	OpGetConstant
	OpSetConstant
	OpGetScopedConstant
	OpSetScopedConstant

	OpClosure

	OpCurrentClosure
	OpSingletonClass

	OpReturn
	OpReturnValue
	OpBlockReturn
	OpNext

	OpSend
	OpSendWithKeywords
	OpSendSetter
	OpSendWithBlock
	OpSendSuper

	OpDefineMethod
	OpDefineSingletonMethod
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
	OpEnterForEach
	OpExitForEach
	OpSetWhileEnd
	OpRedo

	OpMatch
	OpNotMatch

	OpToAry
	OpMultiAssignPrepare
	OpMultiAssignExtract
	OpMultiAssignCheckToAry

	OpDup
	OpSwap
	OpCaseSplatMatch

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
	OpYieldWithSplat

	OpRescue
	OpRescueMatch

	OpRetry
	OpRaise
	OpReraise
	OpThrow
	OpBeginRescue
	OpEndRescue
	OpEnsure
	OpEndEnsure
	OpCatch
	OpEndCatch

	OpExtend
	OpPrepend

	OpAlias
	OpUndef

	OpDefined
	OpDefinedConstant
	OpDefinedMethod
	OpDefinedYield
	OpDefinedSuper

	OpCaseEq

	OpIsA
	OpKindOf

	OpInstanceOf
	OpRespondTo

	OpClassOf

	OpFreeze

	OpSplat
	OpSplatToA
	OpSplatToArray

	OpRange
	OpBlockGiven
	OpRationalNew
	OpPatternCheck
	OpPatternCacheClear

	OpDebug
	OpJumpLocalPresent
	OpLogicalSendAssignment
	OpRegexpOnceGet
	OpRegexpOnceSet
	OpNonLocalReturnValue
	OpRaiseNoMatchingPattern
	OpGetMatchCapture
	OpSetStringEncoding
	OpGetLocalFast
)

type Definition struct {
	Name          string
	OperandWidths []int
}

const (
	ScopedConstAssignPlain = iota
	ScopedConstAssignOr
	ScopedConstAssignAnd
	ScopedConstAssignAdd
)

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
	OpFlipFlopGet:   {"OpFlipFlopGet", []int{2}},
	OpFlipFlopSet:   {"OpFlipFlopSet", []int{2, 1}},

	OpArray:           {"OpArray", []int{4}},
	OpArrayAppend:     {"OpArrayAppend", []int{1}},
	OpHash:            {"OpHash", []int{2}},
	OpHashMerge:       {"OpHashMerge", []int{}},
	OpMarkKeywordHash: {"OpMarkKeywordHash", []int{}},

	OpIndex:                    {"OpIndex", []int{}},
	OpSliceIndex:               {"OpSliceIndex", []int{}},
	OpIndexAssign:              {"OpIndexAssign", []int{}},
	OpIndexCompoundAssign:      {"OpIndexCompoundAssign", []int{2}},
	OpIndexSplatCompoundAssign: {"OpIndexSplatCompoundAssign", []int{2}},

	OpGetGlobal: {"OpGetGlobal", []int{2}},
	OpSetGlobal: {"OpSetGlobal", []int{2}},

	OpGetLocal:     {"OpGetLocal", []int{1}},
	OpSetLocal:     {"OpSetLocal", []int{1}},
	OpGetLocalCell: {"OpGetLocalCell", []int{1}},

	OpGetFree:     {"OpGetFree", []int{1}},
	OpSetFree:     {"OpSetFree", []int{1}},
	OpGetFreeCell: {"OpGetFreeCell", []int{1}},

	OpGetOuter:         {"OpGetOuter", []int{1}},
	OpSetOuter:         {"OpSetOuter", []int{1, 1}},
	OpGetOuterFree:     {"OpGetOuterFree", []int{1}},
	OpSetOuterFree:     {"OpSetOuterFree", []int{1}},
	OpGetOuterCell:     {"OpGetOuterCell", []int{1}},
	OpGetOuterFreeCell: {"OpGetOuterFreeCell", []int{1}},

	OpGetInstanceVar:     {"OpGetInstanceVar", []int{2}},
	OpSetInstanceVar:     {"OpSetInstanceVar", []int{2}},
	OpDefinedInstanceVar: {"OpDefinedInstanceVar", []int{2}},

	OpGetClassVar:      {"OpGetClassVar", []int{2}},
	OpGetClassVarOrNil: {"OpGetClassVarOrNil", []int{2}},
	OpSetClassVar:      {"OpSetClassVar", []int{2}},
	OpDefinedClassVar:  {"OpDefinedClassVar", []int{2}},
	OpDefinedGlobal:    {"OpDefinedGlobal", []int{2}},

	OpGetConstant:       {"OpGetConstant", []int{2}},
	OpSetConstant:       {"OpSetConstant", []int{2, 1}},
	OpGetScopedConstant: {"OpGetScopedConstant", []int{2}},
	OpSetScopedConstant: {"OpSetScopedConstant", []int{2, 1}},

	OpClosure:        {"OpClosure", []int{2, 1}},
	OpCurrentClosure: {"OpCurrentClosure", []int{}},
	OpSingletonClass: {"OpSingletonClass", []int{}},

	OpReturn:      {"OpReturn", []int{}},
	OpReturnValue: {"OpReturnValue", []int{}},
	OpBlockReturn: {"OpBlockReturn", []int{}},
	OpNext:        {"OpNext", []int{2}},

	OpSend:             {"OpSend", []int{2, 1, 1, 1}},
	OpSendWithKeywords: {"OpSendWithKeywords", []int{2, 1, 1, 1}},
	OpSendSetter:       {"OpSendSetter", []int{2, 1, 1, 1}},
	OpSendWithBlock:    {"OpSendWithBlock", []int{2, 1, 1, 2}},
	OpSendSuper:        {"OpSendSuper", []int{2, 1, 1, 1}},

	OpDefineMethod:          {"OpDefineMethod", []int{2}},
	OpDefineSingletonMethod: {"OpDefineSingletonMethod", []int{2}},
	OpDefineClassMethod:     {"OpDefineClassMethod", []int{2}},
	OpDefineFunction:        {"OpDefineFunction", []int{2}},

	OpClass:  {"OpClass", []int{2, 1}},
	OpModule: {"OpModule", []int{2}},

	OpInherited: {"OpInherited", []int{}},
	OpIncluded:  {"OpIncluded", []int{}},
	OpExtended:  {"OpExtended", []int{}},

	OpOpenClass:          {"OpOpenClass", []int{2}},
	OpOpenClassWithSuper: {"OpOpenClassWithSuper", []int{2}},

	OpLambda:       {"OpLambda", []int{2, 1}},
	OpBlock:        {"OpBlock", []int{2}},
	OpBlockWithArg: {"OpBlockWithArg", []int{2, 1}},

	OpBreak:        {"OpBreak", []int{}},
	OpBreakValue:   {"OpBreakValue", []int{2}},
	OpEnterForEach: {"OpEnterForEach", []int{1}},
	OpExitForEach:  {"OpExitForEach", []int{}},
	OpSetWhileEnd:  {"OpSetWhileEnd", []int{2}},
	OpRedo:         {"OpRedo", []int{}},

	OpMatch:    {"OpMatch", []int{}},
	OpNotMatch: {"OpNotMatch", []int{}},

	OpToAry:                 {"OpToAry", []int{}},
	OpMultiAssignPrepare:    {"OpMultiAssignPrepare", []int{}},
	OpMultiAssignExtract:    {"OpMultiAssignExtract", []int{1, 1, 1, 1}},
	OpMultiAssignCheckToAry: {"OpMultiAssignCheckToAry", []int{}},
	OpDup:                   {"OpDup", []int{}},
	OpSwap:                  {"OpSwap", []int{}},
	OpCaseSplatMatch:        {"OpCaseSplatMatch", []int{}},

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
	OpYieldWithSplat: {"OpYieldWithSplat", []int{1, 1}},

	OpRescue:      {"OpRescue", []int{}},
	OpRescueMatch: {"OpRescueMatch", []int{1, 2}},
	OpRetry:       {"OpRetry", []int{}},
	OpRaise:       {"OpRaise", []int{}},
	OpReraise:     {"OpReraise", []int{}},
	OpThrow:       {"OpThrow", []int{}},
	OpBeginRescue: {"OpBeginRescue", []int{2, 2, 2, 2}},
	OpEndRescue:   {"OpEndRescue", []int{}},
	OpEnsure:      {"OpEnsure", []int{}},
	OpEndEnsure:   {"OpEndEnsure", []int{}},
	OpCatch:       {"OpCatch", []int{2}},
	OpEndCatch:    {"OpEndCatch", []int{}},

	OpExtend:  {"OpExtend", []int{}},
	OpPrepend: {"OpPrepend", []int{}},

	OpAlias: {"OpAlias", []int{}},
	OpUndef: {"OpUndef", []int{2}},

	OpDefined:                {"OpDefined", []int{2}},
	OpDefinedConstant:        {"OpDefinedConstant", []int{2}},
	OpDefinedMethod:          {"OpDefinedMethod", []int{2, 1}},
	OpDefinedYield:           {"OpDefinedYield", []int{}},
	OpDefinedSuper:           {"OpDefinedSuper", []int{}},
	OpCaseEq:                 {"OpCaseEq", []int{}},
	OpIsA:                    {"OpIsA", []int{}},
	OpKindOf:                 {"OpKindOf", []int{}},
	OpInstanceOf:             {"OpInstanceOf", []int{}},
	OpRespondTo:              {"OpRespondTo", []int{}},
	OpClassOf:                {"OpClassOf", []int{}},
	OpFreeze:                 {"OpFreeze", []int{}},
	OpSplat:                  {"OpSplat", []int{}},
	OpSplatToA:               {"OpSplatToA", []int{}},
	OpSplatToArray:           {"OpSplatToArray", []int{}},
	OpRange:                  {"OpRange", []int{1, 1, 1}},
	OpBlockGiven:             {"OpBlockGiven", []int{}},
	OpRationalNew:            {"OpRationalNew", []int{}},
	OpPatternCheck:           {"OpPatternCheck", []int{2}},
	OpPatternCacheClear:      {"OpPatternCacheClear", []int{1}},
	OpDebug:                  {"OpDebug", []int{}},
	OpJumpLocalPresent:       {"OpJumpLocalPresent", []int{1, 2}},
	OpLogicalSendAssignment:  {"OpLogicalSendAssignment", []int{2, 2, 1, 1}},
	OpRegexpOnceGet:          {"OpRegexpOnceGet", []int{2, 2}},
	OpRegexpOnceSet:          {"OpRegexpOnceSet", []int{2}},
	OpNonLocalReturnValue:    {"OpNonLocalReturnValue", []int{}},
	OpRaiseNoMatchingPattern: {"OpRaiseNoMatchingPattern", []int{}},
	OpGetMatchCapture:        {"OpGetMatchCapture", []int{2}},
	OpSetStringEncoding:      {"OpSetStringEncoding", []int{2}},
	OpGetLocalFast:           {"OpGetLocalFast", []int{1}},
}

func Lookup(op byte) (Definition, bool) {
	def, ok := definitions[Opcode(op)]
	return def, ok
}
