package vm

import (
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// executeNativePDFClassNew covers the allocation-only constructors that sit
// below PDF::Core::ObjectStore#initialize. ObjectStore creates a large number
// of Reference, Stream and FilterList objects during a render; entering Ruby
// just to bind their fixed ivars is measurable, but the proof must remain
// closed-world because these classes are public Ruby classes.
//
// The exact class pointer, initialize source and method generation are checked
// by the caller's resolved method. A class redefinition, subclass, singleton
// override, unsupported arity or missing dependency deopts to Class#new.
func (vm *VM) executeNativePDFClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || receiver == nil || receiver.Type != object.ValueClass ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || core.AttachedSingletonClass(receiver) != nil {
		return nil, false
	}
	cls, ok := receiver.Data.(*object.Class)
	if !ok || cls == nil {
		return nil, false
	}
	initialize, owner, found := cls.GetMethodWithOwner("initialize")
	if !found || initialize == nil || owner != cls || initialize.DispatchOwner != nil ||
		initialize.Visibility != "" && initialize.Visibility != "public" || initialize.Ruby2Keywords {
		return nil, false
	}
	fn, ok := initialize.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "initialize" {
		return nil, false
	}

	switch cls.Name {
	case "PDF::Core::GraphicState":
		if cls != vm.nativePDFConstructorClass("PDF::Core::GraphicState") || len(args) > 1 ||
			len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || !strings.HasSuffix(fn.SourcePath, "/graphics_state.rb") {
			return nil, false
		}
		previous := (*object.EmeraldValue)(nil)
		if len(args) == 1 {
			previous = args[0]
		}
		return nativePDFGraphicStateValue(cls, previous, false)
	case "PDF::Core::GraphicStateStack":
		if cls != vm.nativePDFConstructorClass("PDF::Core::GraphicStateStack") || len(args) > 1 ||
			len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || !strings.HasSuffix(fn.SourcePath, "/graphics_state.rb") {
			return nil, false
		}
		previous := (*object.EmeraldValue)(nil)
		if len(args) == 1 {
			previous = args[0]
		}
		state, handled := nativePDFGraphicStateValue(vm.nativePDFConstructorClass("PDF::Core::GraphicState"), previous, true)
		if !handled {
			return nil, false
		}
		stack := object.NewObjectValue(cls)
		core.SetDynamicInstanceVar(stack, "@stack", &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{state},
			Class: core.R.Classes["Array"],
		})
		return stack, true
	case "PDF::Core::ObjectStore":
		if cls != vm.nativePDFConstructorClass("PDF::Core::ObjectStore") || len(args) > 1 ||
			len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || !strings.HasSuffix(fn.SourcePath, "/object_store.rb") ||
			!core.HashIndexUsesBuiltinImplementation() || !core.HashStoreUsesBuiltinImplementation() {
			return nil, false
		}
		options := nativePDFEmptyHash()
		if len(args) == 1 {
			if args[0] == nil || args[0].Type != object.ValueHash || args[0].Class != core.R.Classes["Hash"] ||
				core.AttachedSingletonClass(args[0]) != nil {
				return nil, false
			}
			options = args[0]
		}
		info, handled := core.DirectHashIndex(options, nativePDFSymbol("info"))
		if !handled {
			return nil, false
		}
		if info == nil || info.Type == object.ValueNil {
			info = nativePDFEmptyHash()
		}
		if info.Type != object.ValueHash || info.Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(info) != nil {
			return nil, false
		}
		printScaling, handled := core.DirectHashIndex(options, nativePDFSymbol("print_scaling"))
		if !handled {
			return nil, false
		}
		return nativePDFObjectStoreValue(
			cls,
			vm.nativePDFConstructorClass("PDF::Core::Reference"),
			vm.nativePDFConstructorClass("PDF::Core::Stream"),
			vm.nativePDFConstructorClass("PDF::Core::FilterList"),
			info,
			printScaling,
		)
	case "PDF::Core::DocumentState":
		if cls != vm.nativePDFConstructorClass("PDF::Core::DocumentState") || len(args) != 1 ||
			len(fn.Params) != 1 || !strings.HasSuffix(fn.SourcePath, "/document_state.rb") {
			return nil, false
		}
		return vm.nativePDFDocumentStateValue(cls, args[0])
	case "PDF::Core::Renderer":
		if cls != vm.nativePDFConstructorClass("PDF::Core::Renderer") || len(args) != 1 ||
			len(fn.Params) != 1 || !strings.HasSuffix(fn.SourcePath, "/renderer.rb") {
			return nil, false
		}
		return vm.nativePDFRendererValue(cls, args[0])
	case "PDF::Core::Page":
		if cls != vm.nativePDFConstructorClass("PDF::Core::Page") || len(args) < 1 || len(args) > 2 ||
			len(fn.Params) != 2 || len(fn.ParamDefaults) != 2 || !strings.HasSuffix(fn.SourcePath, "/page.rb") ||
			!core.HashIndexUsesBuiltinImplementation() || !core.HashStoreUsesBuiltinImplementation() ||
			!core.ArrayAppendUsesBuiltinImplementation() {
			return nil, false
		}
		options := nativePDFEmptyHash()
		if len(args) == 2 {
			if args[1] == nil || !nativePDFStandardHash(args[1]) {
				return nil, false
			}
			options = args[1]
		}
		return vm.nativePDFPageValue(cls, args[0], options)
	case "PDF::Core::FilterList":
		if cls != vm.nativePDFConstructorClass("PDF::Core::FilterList") || len(args) != 0 ||
			len(fn.Params) != 0 || !strings.HasSuffix(fn.SourcePath, "/filter_list.rb") ||
			core.R.Classes["Array"] == nil {
			return nil, false
		}
		return nativePDFFilterListValue(cls, false), true
	case "PDF::Core::Stream":
		filterClass := vm.nativePDFConstructorClass("PDF::Core::FilterList")
		if cls != vm.nativePDFConstructorClass("PDF::Core::Stream") || len(args) > 1 ||
			len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || !strings.HasSuffix(fn.SourcePath, "/stream.rb") || filterClass == nil ||
			core.R.Classes["Array"] == nil {
			return nil, false
		}
		filterInitialize, filterOwner, filterFound := filterClass.GetMethodWithOwner("initialize")
		if _, ok := pdfConstructorFunction(filterInitialize, filterOwner, filterClass, filterFound, "/filter_list.rb"); !ok {
			return nil, false
		}
		return nativePDFStreamValue(cls, filterClass, args, false), true
	case "PDF::Core::Reference":
		streamClass := vm.nativePDFConstructorClass("PDF::Core::Stream")
		if cls != vm.nativePDFConstructorClass("PDF::Core::Reference") || len(args) != 2 ||
			len(fn.Params) != 2 || !strings.HasSuffix(fn.SourcePath, "/reference.rb") || streamClass == nil {
			return nil, false
		}
		streamInitialize, streamOwner, streamFound := streamClass.GetMethodWithOwner("initialize")
		streamFn, streamOK := pdfConstructorFunction(streamInitialize, streamOwner, streamClass, streamFound, "/stream.rb")
		if !streamOK || streamFn == nil {
			return nil, false
		}
		_ = streamFn
		filterClass := vm.nativePDFConstructorClass("PDF::Core::FilterList")
		if filterClass == nil || core.R.Classes["Array"] == nil {
			return nil, false
		}
		filterInitialize, filterOwner, filterFound := filterClass.GetMethodWithOwner("initialize")
		if _, ok := pdfConstructorFunction(filterInitialize, filterOwner, filterClass, filterFound, "/filter_list.rb"); !ok {
			return nil, false
		}
		return nativePDFReferenceValue(cls, streamClass, filterClass, args[0], args[1], false), true
	default:
		return nil, false
	}
}

func nativePDFHashValue(entries ...[2]*object.EmeraldValue) *object.EmeraldValue {
	hash := &object.RHash{
		Pairs: make(map[*object.EmeraldValue]*object.EmeraldValue, len(entries)),
		Keys:  make([]*object.EmeraldValue, 0, len(entries)),
	}
	for _, entry := range entries {
		hash.Keys = append(hash.Keys, entry[0])
		hash.Pairs[entry[0]] = entry[1]
	}
	return &object.EmeraldValue{Type: object.ValueHash, Data: hash, Class: core.R.Classes["Hash"]}
}

func nativePDFEmptyHash() *object.EmeraldValue {
	return nativePDFHashValue()
}

func nativePDFLookupHashEntry(value *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] {
		return nil, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil {
		return nil, false
	}
	for _, key := range hash.Keys {
		if key == nil || key.Type != object.ValueSymbol {
			continue
		}
		keyName, ok := key.Data.(string)
		if !ok || keyName != name {
			continue
		}
		result, exists := hash.Pairs[key]
		if !exists || result == nil {
			return core.R.NilVal, true
		}
		return result, true
	}
	for key, result := range hash.Pairs {
		if key == nil || key.Type != object.ValueSymbol {
			continue
		}
		keyName, ok := key.Data.(string)
		if ok && keyName == name {
			if result == nil {
				return core.R.NilVal, true
			}
			return result, true
		}
	}
	return core.R.NilVal, false
}

func nativePDFStandardHash(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	data, ok := value.Data.(*object.RHash)
	return ok && data != nil && data.Default == nil && data.DefaultProc == nil && !data.CompareByIdentity
}

func nativePDFFalsy(value *object.EmeraldValue) bool {
	if value == nil || value.Type == object.ValueNil {
		return true
	}
	if value.Type != object.ValueBool {
		return false
	}
	flag, ok := value.Data.(bool)
	return ok && !flag
}

func nativePDFSymbol(name string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]}
}

func nativePDFFrozenString(name string) *object.EmeraldValue {
	value := core.NewStringValue(name)
	value.Frozen = true
	value.Literal = true
	return value
}

func nativePDFCloneString(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type != object.ValueString {
		return nil
	}
	clone := *value
	clone.Cold = value.CloneColdData()
	clone.Frozen = false
	clone.Literal = false
	if len(value.InstanceVars) > 0 {
		clone.InstanceVars = make(map[string]*object.EmeraldValue, len(value.InstanceVars))
		for name, item := range value.InstanceVars {
			clone.InstanceVars[name] = item
		}
	}
	return &clone
}

func nativePDFCloneHash(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] {
		return nil
	}
	source, ok := value.Data.(*object.RHash)
	if !ok || source == nil {
		return nil
	}
	clone := &object.RHash{
		Pairs:             make(map[*object.EmeraldValue]*object.EmeraldValue, len(source.Pairs)),
		Keys:              append([]*object.EmeraldValue(nil), source.Keys...),
		Hashes:            make(map[*object.EmeraldValue]int64, len(source.Hashes)),
		Default:           source.Default,
		DefaultProc:       source.DefaultProc,
		CompareByIdentity: source.CompareByIdentity,
	}
	for key, item := range source.Pairs {
		clone.Pairs[key] = item
	}
	for key, code := range source.Hashes {
		clone.Hashes[key] = code
	}
	return &object.EmeraldValue{Type: object.ValueHash, Data: clone, Class: value.Class}
}

func nativePDFGraphicStateValue(cls *object.Class, previous *object.EmeraldValue, track bool) (*object.EmeraldValue, bool) {
	if cls == nil || core.R.Classes["Hash"] == nil || core.R.Classes["String"] == nil || core.R.Classes["Symbol"] == nil {
		return nil, false
	}
	value := object.NewObjectValue(cls)
	if previous == nil || previous.Type == object.ValueNil || previous.Type == object.ValueBool && !previous.Data.(bool) {
		core.SetDynamicInstanceVar(value, "@color_space", nativePDFHashValue())
		core.SetDynamicInstanceVar(value, "@fill_color", nativePDFFrozenString("000000"))
		core.SetDynamicInstanceVar(value, "@stroke_color", nativePDFFrozenString("000000"))
		core.SetDynamicInstanceVar(value, "@dash", nativePDFHashValue(
			[2]*object.EmeraldValue{nativePDFSymbol("dash"), core.R.NilVal},
			[2]*object.EmeraldValue{nativePDFSymbol("space"), core.R.NilVal},
			[2]*object.EmeraldValue{nativePDFSymbol("phase"), core.NewIntegerValue(0)},
		))
		core.SetDynamicInstanceVar(value, "@cap_style", nativePDFSymbol("butt"))
		core.SetDynamicInstanceVar(value, "@join_style", nativePDFSymbol("miter"))
		core.SetDynamicInstanceVar(value, "@line_width", core.NewIntegerValue(1))
	} else {
		if previous.Type != object.ValueObject || previous.Class != cls || core.AttachedSingletonClass(previous) != nil {
			return nil, false
		}
		colorSpace := core.DynamicInstanceVar(previous, "@color_space")
		fillColor := core.DynamicInstanceVar(previous, "@fill_color")
		strokeColor := core.DynamicInstanceVar(previous, "@stroke_color")
		dash := core.DynamicInstanceVar(previous, "@dash")
		capStyle := core.DynamicInstanceVar(previous, "@cap_style")
		joinStyle := core.DynamicInstanceVar(previous, "@join_style")
		lineWidth := core.DynamicInstanceVar(previous, "@line_width")
		if colorSpace == nil || fillColor == nil || strokeColor == nil || dash == nil || capStyle == nil || joinStyle == nil || lineWidth == nil ||
			colorSpace.Type != object.ValueHash || colorSpace.Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(colorSpace) != nil ||
			fillColor.Type != object.ValueString || fillColor.Class != core.R.Classes["String"] || core.AttachedSingletonClass(fillColor) != nil ||
			strokeColor.Type != object.ValueString || strokeColor.Class != core.R.Classes["String"] || core.AttachedSingletonClass(strokeColor) != nil ||
			dash.Type != object.ValueHash || dash.Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(dash) != nil {
			return nil, false
		}
		colorSpaceCopy := nativePDFCloneHash(colorSpace)
		dashCopy := nativePDFCloneHash(dash)
		fillCopy := nativePDFCloneString(fillColor)
		strokeCopy := nativePDFCloneString(strokeColor)
		if colorSpaceCopy == nil || dashCopy == nil || fillCopy == nil || strokeCopy == nil {
			return nil, false
		}
		core.SetDynamicInstanceVar(value, "@color_space", colorSpaceCopy)
		core.SetDynamicInstanceVar(value, "@fill_color", fillCopy)
		core.SetDynamicInstanceVar(value, "@stroke_color", strokeCopy)
		core.SetDynamicInstanceVar(value, "@dash", dashCopy)
		core.SetDynamicInstanceVar(value, "@cap_style", capStyle)
		core.SetDynamicInstanceVar(value, "@join_style", joinStyle)
		core.SetDynamicInstanceVar(value, "@line_width", lineWidth)
	}
	if track {
		core.TrackObjectSpaceValue(value)
	}
	return value, true
}

func nativePDFObjectStoreValue(cls, referenceClass, streamClass, filterClass *object.Class, info, printScaling *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if cls == nil || referenceClass == nil || streamClass == nil || filterClass == nil || !nativePDFStandardHash(info) || printScaling == nil ||
		!core.HashIndexUsesBuiltinImplementation() || !core.HashStoreUsesBuiltinImplementation() {
		return nil, false
	}
	objects := nativePDFEmptyHash()
	identifiers := nativePDFArrayValue()
	store := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(store, "@objects", objects)
	core.SetDynamicInstanceVar(store, "@identifiers", identifiers)

	add := func(identifier int64, data *object.EmeraldValue) (*object.EmeraldValue, bool) {
		ref := nativePDFReferenceValue(referenceClass, streamClass, filterClass, core.NewIntegerValue(identifier), data, true)
		if !core.StoreHashValue(objects, core.NewIntegerValue(identifier), ref) || !core.AppendArrayValue(identifiers, core.NewIntegerValue(identifier)) {
			return nil, false
		}
		return ref, true
	}
	if _, ok := add(1, info); !ok {
		return nil, false
	}
	catalog := nativePDFHashValue([2]*object.EmeraldValue{nativePDFSymbol("Type"), nativePDFSymbol("Catalog")})
	_, ok := add(2, catalog)
	if !ok {
		return nil, false
	}
	if printScaling.Type == object.ValueSymbol && printScaling.Data == "none" {
		preferences := nativePDFHashValue([2]*object.EmeraldValue{nativePDFSymbol("PrintScaling"), nativePDFSymbol("None")})
		if !core.StoreHashValue(catalog, nativePDFSymbol("ViewerPreferences"), preferences) {
			return nil, false
		}
	}
	pagesData := nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("Type"), nativePDFSymbol("Pages")},
		[2]*object.EmeraldValue{nativePDFSymbol("Count"), core.NewIntegerValue(0)},
		[2]*object.EmeraldValue{nativePDFSymbol("Kids"), nativePDFArrayValue()},
	)
	pages, ok := add(3, pagesData)
	if !ok {
		return nil, false
	}
	if !core.StoreHashValue(catalog, nativePDFSymbol("Pages"), pages) {
		return nil, false
	}
	core.SetDynamicInstanceVar(store, "@info", core.NewIntegerValue(1))
	core.SetDynamicInstanceVar(store, "@root", core.NewIntegerValue(2))
	return store, true
}

func nativePDFFetch(options *object.EmeraldValue, name string, defaultValue *object.EmeraldValue) *object.EmeraldValue {
	value, found := nativePDFLookupHashEntry(options, name)
	if !found || value == nil {
		return defaultValue
	}
	return value
}

func (vm *VM) nativePDFDocumentStateValue(cls *object.Class, options *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || cls == nil || !nativePDFStandardHash(options) || options.Frozen ||
		!core.HashIndexUsesBuiltinImplementation() || !core.HashStoreUsesBuiltinImplementation() {
		return nil, false
	}
	info, found := nativePDFLookupHashEntry(options, "info")
	replaceInfo := !found || nativePDFFalsy(info)
	if replaceInfo {
		info = nativePDFEmptyHash()
	}
	if !nativePDFStandardHash(info) {
		return nil, false
	}
	creator, creatorFound := nativePDFLookupHashEntry(info, "Creator")
	producer, producerFound := nativePDFLookupHashEntry(info, "Producer")
	replaceCreator := !creatorFound || nativePDFFalsy(creator)
	replaceProducer := !producerFound || nativePDFFalsy(producer)
	if (replaceCreator || replaceProducer) && info.Frozen {
		return nil, false
	}
	if replaceInfo && options.Frozen {
		return nil, false
	}
	printScaling, found := nativePDFLookupHashEntry(options, "print_scaling")
	if !found {
		printScaling = core.R.NilVal
	}
	storeClass := vm.nativePDFConstructorClass("PDF::Core::ObjectStore")
	store, handled := nativePDFObjectStoreValue(
		storeClass,
		vm.nativePDFConstructorClass("PDF::Core::Reference"),
		vm.nativePDFConstructorClass("PDF::Core::Stream"),
		vm.nativePDFConstructorClass("PDF::Core::FilterList"),
		info,
		printScaling,
	)
	if !handled {
		return nil, false
	}
	if replaceInfo && !core.StoreHashValue(options, nativePDFSymbol("info"), info) {
		return nil, false
	}
	if replaceCreator && !core.StoreHashValue(info, nativePDFSymbol("Creator"), nativePDFFrozenString("Prawn")) {
		return nil, false
	}
	if replaceProducer && !core.StoreHashValue(info, nativePDFSymbol("Producer"), nativePDFFrozenString("Prawn")) {
		return nil, false
	}
	trailer, found := nativePDFLookupHashEntry(options, "trailer")
	if !found || trailer == nil {
		trailer = nativePDFEmptyHash()
	}
	compress := nativePDFFetch(options, "compress", core.R.FalseVal)
	encrypt := nativePDFFetch(options, "encrypt", core.R.FalseVal)
	skipEncoding := nativePDFFetch(options, "skip_encoding", core.R.FalseVal)
	encryptionKey := nativePDFFetch(options, "encryption_key", core.R.NilVal)
	state := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(state, "@store", store)
	core.SetDynamicInstanceVar(state, "@version", core.NewFloatValue(1.3))
	core.SetDynamicInstanceVar(state, "@pages", nativePDFArrayValue())
	core.SetDynamicInstanceVar(state, "@page", core.R.NilVal)
	core.SetDynamicInstanceVar(state, "@trailer", trailer)
	core.SetDynamicInstanceVar(state, "@compress", compress)
	core.SetDynamicInstanceVar(state, "@encrypt", encrypt)
	core.SetDynamicInstanceVar(state, "@encryption_key", encryptionKey)
	core.SetDynamicInstanceVar(state, "@skip_encoding", skipEncoding)
	core.SetDynamicInstanceVar(state, "@before_render_callbacks", nativePDFArrayValue())
	core.SetDynamicInstanceVar(state, "@on_page_create_callback", core.R.NilVal)
	core.TrackObjectSpaceValue(store)
	return state, true
}

func (vm *VM) nativePDFRendererValue(cls *object.Class, state *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || cls == nil || state == nil || state.Type != object.ValueObject ||
		state.Class != vm.nativePDFConstructorClass("PDF::Core::DocumentState") || core.AttachedSingletonClass(state) != nil {
		return nil, false
	}
	pages := core.DynamicInstanceVar(state, "@pages")
	store := core.DynamicInstanceVar(state, "@store")
	page := core.DynamicInstanceVar(state, "@page")
	pageItems, pagesOK := func() ([]*object.EmeraldValue, bool) {
		if pages == nil || pages.Type != object.ValueArray || pages.Class != core.R.Classes["Array"] ||
			core.AttachedSingletonClass(pages) != nil {
			return nil, false
		}
		items, ok := pages.Data.([]*object.EmeraldValue)
		return items, ok
	}()
	if !pagesOK || len(pageItems) != 0 || store == nil || store.Type != object.ValueObject ||
		store.Class != vm.nativePDFConstructorClass("PDF::Core::ObjectStore") || page == nil || page.Type != object.ValueNil ||
		!nativePDFObjectStoreIsEmpty(store, vm.nativePDFConstructorClass("PDF::Core::Reference")) {
		return nil, false
	}
	minVersion := core.DynamicInstanceVar(store, "@min_version")
	if minVersion != nil && minVersion.Type != object.ValueNil {
		return nil, false
	}
	renderer := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(renderer, "@state", state)
	core.SetDynamicInstanceVar(renderer, "@page_number", core.NewIntegerValue(0))
	return renderer, true
}

func nativePDFObjectStoreIsEmpty(store *object.EmeraldValue, expectedReferenceClass *object.Class) bool {
	if store == nil || store.Type != object.ValueObject || expectedReferenceClass == nil || core.AttachedSingletonClass(store) != nil {
		return false
	}
	objects := core.DynamicInstanceVar(store, "@objects")
	rootID := core.DynamicInstanceVar(store, "@root")
	if objects == nil || !nativePDFStandardHash(objects) || rootID == nil || rootID.Type != object.ValueInteger {
		return false
	}
	root, safe := core.DirectHashIndex(objects, rootID)
	if !safe || root == nil || root.Type != object.ValueObject ||
		root.Class != expectedReferenceClass || core.AttachedSingletonClass(root) != nil {
		return false
	}
	rootData := core.DynamicInstanceVar(root, "@data")
	pages, found := nativePDFLookupHashEntry(rootData, "Pages")
	if !found || pages == nil || pages.Type != object.ValueObject ||
		pages.Class != expectedReferenceClass || core.AttachedSingletonClass(pages) != nil {
		return false
	}
	pagesData := core.DynamicInstanceVar(pages, "@data")
	count, countFound := nativePDFLookupHashEntry(pagesData, "Count")
	kids, kidsFound := nativePDFLookupHashEntry(pagesData, "Kids")
	if !countFound || count == nil || count.Type != object.ValueInteger || count.Data != int64(0) ||
		!kidsFound || kids == nil || kids.Type != object.ValueArray || kids.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(kids) != nil {
		return false
	}
	items, ok := kids.Data.([]*object.EmeraldValue)
	return ok && len(items) == 0
}

func nativePDFObjectStorePagesReference(store *object.EmeraldValue, expectedReferenceClass *object.Class) (*object.EmeraldValue, bool) {
	if store == nil || store.Type != object.ValueObject || core.AttachedSingletonClass(store) != nil || expectedReferenceClass == nil {
		return nil, false
	}
	objects := core.DynamicInstanceVar(store, "@objects")
	rootID := core.DynamicInstanceVar(store, "@root")
	if objects == nil || !nativePDFStandardHash(objects) || rootID == nil || rootID.Type != object.ValueInteger {
		return nil, false
	}
	root, safe := core.DirectHashIndex(objects, rootID)
	if !safe || root == nil || root.Type != object.ValueObject || root.Class != expectedReferenceClass ||
		core.AttachedSingletonClass(root) != nil {
		return nil, false
	}
	rootData := core.DynamicInstanceVar(root, "@data")
	pages, found := nativePDFLookupHashEntry(rootData, "Pages")
	if !found || pages == nil || pages.Type != object.ValueObject || pages.Class != expectedReferenceClass ||
		core.AttachedSingletonClass(pages) != nil {
		return nil, false
	}
	return pages, true
}

func nativePDFStoreReference(store *object.EmeraldValue, referenceClass, streamClass, filterClass *object.Class, data *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if store == nil || store.Type != object.ValueObject || core.AttachedSingletonClass(store) != nil ||
		referenceClass == nil || streamClass == nil || filterClass == nil || data == nil {
		return nil, false
	}
	objects := core.DynamicInstanceVar(store, "@objects")
	identifiers := core.DynamicInstanceVar(store, "@identifiers")
	if objects == nil || !nativePDFStandardHash(objects) || identifiers == nil || identifiers.Type != object.ValueArray ||
		identifiers.Class != core.R.Classes["Array"] || identifiers.Frozen || core.AttachedSingletonClass(identifiers) != nil {
		return nil, false
	}
	items, ok := identifiers.Data.([]*object.EmeraldValue)
	if !ok {
		return nil, false
	}
	identifier := core.NewIntegerValue(int64(len(items) + 1))
	reference := nativePDFReferenceValue(referenceClass, streamClass, filterClass, identifier, data, true)
	if !core.StoreHashValue(objects, identifier, reference) || !core.AppendArrayValue(identifiers, identifier) {
		return nil, false
	}
	return reference, true
}

func nativePDFLetterBoxValue() *object.EmeraldValue {
	return &object.EmeraldValue{
		Type: object.ValueArray,
		Data: []*object.EmeraldValue{
			core.NewIntegerValue(0),
			core.NewIntegerValue(0),
			core.NewFloatValue(612),
			core.NewFloatValue(792),
		},
		Class: core.R.Classes["Array"],
	}
}

func nativePDFPageMargins() *object.EmeraldValue {
	return nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("left"), core.NewIntegerValue(36)},
		[2]*object.EmeraldValue{nativePDFSymbol("right"), core.NewIntegerValue(36)},
		[2]*object.EmeraldValue{nativePDFSymbol("top"), core.NewIntegerValue(36)},
		[2]*object.EmeraldValue{nativePDFSymbol("bottom"), core.NewIntegerValue(36)},
	)
}

func (vm *VM) nativePDFPageValue(cls *object.Class, document, options *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || cls == nil || document == nil || document.Type != object.ValueObject || options == nil ||
		!nativePDFStandardHash(options) || core.AttachedSingletonClass(document) != nil {
		return nil, false
	}
	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || document.Class != documentClass {
		return nil, false
	}
	state := core.DynamicInstanceVar(document, "@state")
	stateClass := vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	storeClass := vm.nativePDFConstructorClass("PDF::Core::ObjectStore")
	if state == nil || state.Type != object.ValueObject || state.Class != stateClass || core.AttachedSingletonClass(state) != nil ||
		storeClass == nil {
		return nil, false
	}
	store := core.DynamicInstanceVar(state, "@store")
	if store == nil || store.Type != object.ValueObject || store.Class != storeClass || core.AttachedSingletonClass(store) != nil {
		return nil, false
	}
	referenceClass := vm.nativePDFConstructorClass("PDF::Core::Reference")
	streamClass := vm.nativePDFConstructorClass("PDF::Core::Stream")
	filterClass := vm.nativePDFConstructorClass("PDF::Core::FilterList")
	graphicClass := vm.nativePDFConstructorClass("PDF::Core::GraphicState")
	stackClass := vm.nativePDFConstructorClass("PDF::Core::GraphicStateStack")
	if referenceClass == nil || streamClass == nil || filterClass == nil || graphicClass == nil || stackClass == nil {
		return nil, false
	}
	for dependency, source := range map[*object.Class]string{
		referenceClass: "/reference.rb",
		streamClass:    "/stream.rb",
		filterClass:    "/filter_list.rb",
		graphicClass:   "/graphics_state.rb",
		stackClass:     "/graphics_state.rb",
	} {
		method, owner, present := dependency.GetMethodWithOwner("initialize")
		if _, valid := pdfConstructorFunction(method, owner, dependency, present, source); !valid {
			return nil, false
		}
	}
	pages, ok := nativePDFObjectStorePagesReference(store, referenceClass)
	if !ok {
		return nil, false
	}
	zeroIndents, found := vm.qualifiedConstantValue("PDF::Core::Page::ZERO_INDENTS")
	if !found || !nativePDFStandardHash(zeroIndents) {
		return nil, false
	}

	margins := nativePDFFetch(options, "margins", core.R.NilVal)
	if nativePDFFalsy(margins) {
		margins = nativePDFPageMargins()
	}
	if !nativePDFStandardHash(margins) {
		return nil, false
	}
	boxValues := make(map[string]*object.EmeraldValue, 4)
	for _, name := range []string{"crops", "bleeds", "trims", "art_indents"} {
		value := nativePDFFetch(options, name, core.R.NilVal)
		if nativePDFFalsy(value) {
			value = zeroIndents
		}
		if !nativePDFStandardHash(value) {
			return nil, false
		}
		boxValues[name] = value
	}
	size := nativePDFFetch(options, "size", core.R.NilVal)
	if nativePDFFalsy(size) {
		size = nativePDFFrozenString("LETTER")
	}
	if size.Type != object.ValueString || size.Class != core.R.Classes["String"] || core.AttachedSingletonClass(size) != nil ||
		size.Data != "LETTER" {
		return nil, false
	}
	layout := nativePDFFetch(options, "layout", core.R.NilVal)
	if nativePDFFalsy(layout) {
		layout = nativePDFSymbol("portrait")
	}
	if layout.Type != object.ValueSymbol || layout.Class != core.R.Classes["Symbol"] || layout.Data != "portrait" {
		return nil, false
	}
	previousState := nativePDFFetch(options, "graphic_state", core.R.NilVal)
	stateValue, handled := nativePDFGraphicStateValue(graphicClass, previousState, true)
	if !handled {
		return nil, false
	}
	stack := object.NewObjectValue(stackClass)
	stackValues := nativePDFArrayValue()
	if !core.AppendArrayValue(stackValues, stateValue) {
		return nil, false
	}
	core.SetDynamicInstanceVar(stack, "@stack", stackValues)
	core.TrackObjectSpaceValue(stack)

	contentData := nativePDFEmptyHash()
	content, handled := nativePDFStoreReference(store, referenceClass, streamClass, filterClass, contentData)
	if !handled {
		return nil, false
	}
	contentStream := core.DynamicInstanceVar(content, "@stream")
	if contentStream == nil || contentStream.Type != object.ValueObject || contentStream.Class != streamClass {
		return nil, false
	}
	core.SetDynamicInstanceVar(contentStream, "@stream", core.NewStringValue("q\n"))
	core.SetDynamicInstanceVar(contentStream, "@filtered_stream", core.R.NilVal)

	pageData := nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("Type"), nativePDFSymbol("Page")},
		[2]*object.EmeraldValue{nativePDFSymbol("Parent"), pages},
		[2]*object.EmeraldValue{nativePDFSymbol("MediaBox"), nativePDFLetterBoxValue()},
		[2]*object.EmeraldValue{nativePDFSymbol("CropBox"), nativePDFLetterBoxValue()},
		[2]*object.EmeraldValue{nativePDFSymbol("BleedBox"), nativePDFLetterBoxValue()},
		[2]*object.EmeraldValue{nativePDFSymbol("TrimBox"), nativePDFLetterBoxValue()},
		[2]*object.EmeraldValue{nativePDFSymbol("ArtBox"), nativePDFLetterBoxValue()},
		[2]*object.EmeraldValue{nativePDFSymbol("Contents"), content},
	)
	dictionary, handled := nativePDFStoreReference(store, referenceClass, streamClass, filterClass, pageData)
	if !handled {
		return nil, false
	}
	procSet := nativePDFArrayValue()
	for _, name := range []string{"PDF", "Text", "ImageB", "ImageC", "ImageI"} {
		if !core.AppendArrayValue(procSet, nativePDFSymbol(name)) {
			return nil, false
		}
	}
	resources := nativePDFEmptyHash()
	if !core.StoreHashValue(resources, nativePDFSymbol("ProcSet"), procSet) || !core.StoreHashValue(pageData, nativePDFSymbol("Resources"), resources) {
		return nil, false
	}
	contentID := core.DynamicInstanceVar(content, "@identifier")
	dictionaryID := core.DynamicInstanceVar(dictionary, "@identifier")
	if contentID == nil || dictionaryID == nil {
		return nil, false
	}
	page := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(page, "@art_indents", boxValues["art_indents"])
	core.SetDynamicInstanceVar(page, "@bleeds", boxValues["bleeds"])
	core.SetDynamicInstanceVar(page, "@crops", boxValues["crops"])
	core.SetDynamicInstanceVar(page, "@trims", boxValues["trims"])
	core.SetDynamicInstanceVar(page, "@margins", margins)
	core.SetDynamicInstanceVar(page, "@document", document)
	core.SetDynamicInstanceVar(page, "@stack", stack)
	core.SetDynamicInstanceVar(page, "@size", size)
	core.SetDynamicInstanceVar(page, "@layout", layout)
	core.SetDynamicInstanceVar(page, "@stamp_stream", core.R.NilVal)
	core.SetDynamicInstanceVar(page, "@stamp_dictionary", core.R.NilVal)
	core.SetDynamicInstanceVar(page, "@content", contentID)
	core.SetDynamicInstanceVar(page, "@dictionary", dictionaryID)
	return page, true
}

func (vm *VM) executeNativePDFObjectStorePush(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class != vm.nativePDFConstructorClass("PDF::Core::ObjectStore") ||
		len(args) != 1 && len(args) != 2 || core.AttachedSingletonClass(receiver) != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "push" && fn.Name != "ref" || !strings.HasSuffix(fn.SourcePath, "/object_store.rb") {
		return nil, false
	}
	objects := core.DynamicInstanceVar(receiver, "@objects")
	identifiers := core.DynamicInstanceVar(receiver, "@identifiers")
	if objects == nil || objects.Type != object.ValueHash || objects.Class != core.R.Classes["Hash"] ||
		objects.Frozen || core.AttachedSingletonClass(objects) != nil ||
		identifiers == nil || identifiers.Type != object.ValueArray || identifiers.Class != core.R.Classes["Array"] ||
		identifiers.Frozen || core.AttachedSingletonClass(identifiers) != nil {
		return nil, false
	}
	if _, ok := identifiers.Data.([]*object.EmeraldValue); !ok {
		return nil, false
	}
	if fn.Name == "ref" {
		if len(args) != 1 {
			return nil, false
		}
		elements := identifiers.Data.([]*object.EmeraldValue)
		pushArgs := [2]*object.EmeraldValue{core.NewIntegerValue(int64(len(elements) + 1)), args[0]}
		args = pushArgs[:]
	}
	referenceClass := vm.nativePDFConstructorClass("PDF::Core::Reference")
	var reference *object.EmeraldValue
	if len(args) == 1 {
		reference = args[0]
		if reference == nil || reference.Type != object.ValueObject || reference.Class != referenceClass ||
			core.AttachedSingletonClass(reference) != nil {
			return nil, false
		}
	} else {
		if referenceClass == nil || args[0] == nil || args[0].Type != object.ValueInteger {
			return nil, false
		}
		streamClass := vm.nativePDFConstructorClass("PDF::Core::Stream")
		filterClass := vm.nativePDFConstructorClass("PDF::Core::FilterList")
		if streamClass == nil || filterClass == nil {
			return nil, false
		}
		streamInitialize, streamOwner, streamFound := streamClass.GetMethodWithOwner("initialize")
		streamFn, streamOK := pdfConstructorFunction(streamInitialize, streamOwner, streamClass, streamFound, "/stream.rb")
		if !streamOK || streamFn == nil {
			return nil, false
		}
		filterInitialize, filterOwner, filterFound := filterClass.GetMethodWithOwner("initialize")
		if _, ok := pdfConstructorFunction(filterInitialize, filterOwner, filterClass, filterFound, "/filter_list.rb"); !ok {
			return nil, false
		}
		reference = nativePDFReferenceValue(referenceClass, streamClass, filterClass, args[0], args[1], true)
	}
	identifier := core.DynamicInstanceVar(reference, "@identifier")
	if identifier == nil || identifier.Type != object.ValueInteger || identifier.Class != core.R.Classes["Integer"] ||
		core.AttachedSingletonClass(identifier) != nil {
		return nil, false
	}
	if _, safe := core.DirectHashIndex(objects, identifier); !safe {
		return nil, false
	}
	if !core.StoreHashValue(objects, identifier, reference) {
		return nil, false
	}
	if !core.AppendArrayValue(identifiers, identifier) {
		return nil, false
	}
	return reference, true
}

func (vm *VM) nativePDFConstructorClass(name string) *object.Class {
	if vm == nil {
		return nil
	}
	generation := object.CurrentConstantGeneration()
	if vm.nativePDFConstructorConstantGeneration != generation {
		vm.nativePDFConstructorConstantGeneration = generation
		vm.nativePDFFilterListClass = vm.resolveNativePDFConstructorClass("PDF::Core::FilterList")
		vm.nativePDFStreamClass = vm.resolveNativePDFConstructorClass("PDF::Core::Stream")
		vm.nativePDFReferenceClass = vm.resolveNativePDFConstructorClass("PDF::Core::Reference")
		vm.nativePDFObjectStoreClass = vm.resolveNativePDFConstructorClass("PDF::Core::ObjectStore")
		vm.nativePDFGraphicStateClass = vm.resolveNativePDFConstructorClass("PDF::Core::GraphicState")
		vm.nativePDFGraphicStateStackClass = vm.resolveNativePDFConstructorClass("PDF::Core::GraphicStateStack")
		vm.nativePDFDocumentStateClass = vm.resolveNativePDFConstructorClass("PDF::Core::DocumentState")
		vm.nativePDFRendererClass = vm.resolveNativePDFConstructorClass("PDF::Core::Renderer")
		vm.nativePDFPageClass = vm.resolveNativePDFConstructorClass("PDF::Core::Page")
	}
	switch name {
	case "PDF::Core::FilterList":
		return vm.nativePDFFilterListClass
	case "PDF::Core::Stream":
		return vm.nativePDFStreamClass
	case "PDF::Core::Reference":
		return vm.nativePDFReferenceClass
	case "PDF::Core::ObjectStore":
		return vm.nativePDFObjectStoreClass
	case "PDF::Core::GraphicState":
		return vm.nativePDFGraphicStateClass
	case "PDF::Core::GraphicStateStack":
		return vm.nativePDFGraphicStateStackClass
	case "PDF::Core::DocumentState":
		return vm.nativePDFDocumentStateClass
	case "PDF::Core::Renderer":
		return vm.nativePDFRendererClass
	case "PDF::Core::Page":
		return vm.nativePDFPageClass
	default:
		return nil
	}
}

func (vm *VM) resolveNativePDFConstructorClass(name string) *object.Class {
	value, found := vm.qualifiedConstantValue(name)
	if !found || value == nil || value.Type != object.ValueClass {
		return nil
	}
	cls, _ := value.Data.(*object.Class)
	return cls
}

func pdfConstructorFunction(method *object.Method, owner, expectedOwner *object.Class, found bool, sourceSuffix string) (*object.Function, bool) {
	if !found || method == nil || owner == nil || owner != expectedOwner || method.DispatchOwner != nil ||
		method.Visibility != "" && method.Visibility != "public" || method.Ruby2Keywords {
		return nil, false
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "initialize" || !strings.HasSuffix(fn.SourcePath, sourceSuffix) {
		return nil, false
	}
	return fn, true
}

func nativePDFArrayValue() *object.EmeraldValue {
	value := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: core.R.Classes["Array"]}
	core.TrackObjectSpaceValue(value)
	return value
}

func nativePDFFilterListValue(cls *object.Class, track bool) *object.EmeraldValue {
	value := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(value, "@list", nativePDFArrayValue())
	if track {
		core.TrackObjectSpaceValue(value)
	}
	return value
}

func nativePDFStreamValue(cls, filterClass *object.Class, args []*object.EmeraldValue, track bool) *object.EmeraldValue {
	value := object.NewObjectValue(cls)
	stream := core.R.NilVal
	if len(args) == 1 && args[0] != nil {
		stream = args[0]
	}
	filtered := core.NewStringValue("")
	core.TrackObjectSpaceValue(filtered)
	core.SetDynamicInstanceVar(value, "@filtered_stream", filtered)
	core.SetDynamicInstanceVar(value, "@stream", stream)
	core.SetDynamicInstanceVar(value, "@filters", nativePDFFilterListValue(filterClass, true))
	if track {
		core.TrackObjectSpaceValue(value)
	}
	return value
}

func nativePDFReferenceValue(cls, streamClass, filterClass *object.Class, identifier, data *object.EmeraldValue, track bool) *object.EmeraldValue {
	value := object.NewObjectValue(cls)
	core.SetDynamicInstanceVar(value, "@identifier", identifier)
	core.SetDynamicInstanceVar(value, "@gen", core.NewIntegerValue(0))
	core.SetDynamicInstanceVar(value, "@data", data)
	core.SetDynamicInstanceVar(value, "@stream", nativePDFStreamValue(streamClass, filterClass, nil, true))
	if track {
		core.TrackObjectSpaceValue(value)
	}
	return value
}
