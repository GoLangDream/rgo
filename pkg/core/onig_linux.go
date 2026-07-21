//go:build linux && cgo

package core

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

typedef unsigned char OnigUChar;
typedef unsigned int OnigOptionType;
typedef void* OnigRegex;
typedef void* OnigEncoding;
typedef void OnigSyntaxType;

typedef struct {
  OnigEncoding enc;
  OnigUChar* par;
  OnigUChar* par_end;
} OnigErrorInfo;

typedef struct {
  int allocated;
  int num_regs;
  int* beg;
  int* end;
  void* history_root;
} OnigRegion;

typedef int (*onig_new_fn)(OnigRegex*, const OnigUChar*, const OnigUChar*, OnigOptionType, OnigEncoding, OnigSyntaxType*, OnigErrorInfo*);
typedef void (*onig_free_fn)(OnigRegex);
typedef OnigRegion* (*onig_region_new_fn)(void);
typedef void (*onig_region_free_fn)(OnigRegion*, int);
typedef int (*onig_search_fn)(OnigRegex, const OnigUChar*, const OnigUChar*, const OnigUChar*, const OnigUChar*, OnigRegion*, OnigOptionType);
typedef int (*onig_error_code_to_str_fn)(OnigUChar*, int, ...);

typedef struct {
  void* handle;
  onig_new_fn new_regex;
  onig_free_fn free_regex;
  onig_region_new_fn region_new;
  onig_region_free_fn region_free;
  onig_search_fn search;
  onig_error_code_to_str_fn error_string;
  OnigEncoding utf8;
  OnigSyntaxType* ruby_syntax;
  int loaded;
} rgo_onig_api;

static rgo_onig_api rgo_onig;

static int rgo_load_onig(void) {
  if (rgo_onig.loaded != 0) return rgo_onig.loaded > 0;
  const char* names[] = {"libonig.so.5", "libonig.so", NULL};
  for (int i = 0; names[i] != NULL && rgo_onig.handle == NULL; i++) {
    rgo_onig.handle = dlopen(names[i], RTLD_LAZY | RTLD_LOCAL);
  }
  if (rgo_onig.handle == NULL) {
    rgo_onig.loaded = -1;
    return 0;
  }
#define RGO_DLSYM(field, type, name) rgo_onig.field = (type)dlsym(rgo_onig.handle, name)
  RGO_DLSYM(new_regex, onig_new_fn, "onig_new");
  RGO_DLSYM(free_regex, onig_free_fn, "onig_free");
  RGO_DLSYM(region_new, onig_region_new_fn, "onig_region_new");
  RGO_DLSYM(region_free, onig_region_free_fn, "onig_region_free");
  RGO_DLSYM(search, onig_search_fn, "onig_search");
  RGO_DLSYM(error_string, onig_error_code_to_str_fn, "onig_error_code_to_str");
  rgo_onig.utf8 = (OnigEncoding)dlsym(rgo_onig.handle, "OnigEncodingUTF8");
  rgo_onig.ruby_syntax = (OnigSyntaxType*)dlsym(rgo_onig.handle, "OnigSyntaxRuby");
#undef RGO_DLSYM
  if (rgo_onig.new_regex == NULL || rgo_onig.free_regex == NULL ||
      rgo_onig.region_new == NULL || rgo_onig.region_free == NULL ||
      rgo_onig.search == NULL || rgo_onig.utf8 == NULL || rgo_onig.ruby_syntax == NULL) {
    dlclose(rgo_onig.handle);
    memset(&rgo_onig, 0, sizeof(rgo_onig));
    rgo_onig.loaded = -1;
    return 0;
  }
  rgo_onig.loaded = 1;
  return 1;
}

static int rgo_onig_search(const unsigned char* pattern, int pattern_len,
                           const unsigned char* source, int source_len,
                           unsigned int options, int** begins, int** ends,
                           int* count, char** error_message) {
  *begins = NULL;
  *ends = NULL;
  *count = 0;
  *error_message = NULL;
  if (!rgo_load_onig()) return -2;

  OnigRegex regex = NULL;
  OnigErrorInfo error_info;
  int rc = rgo_onig.new_regex(&regex, pattern, pattern + pattern_len, options,
                              rgo_onig.utf8, rgo_onig.ruby_syntax, &error_info);
  if (rc < 0) {
    if (rgo_onig.error_string != NULL) {
      unsigned char buffer[256];
      rgo_onig.error_string(buffer, rc, &error_info);
      *error_message = strdup((char*)buffer);
    }
    return -1;
  }

  OnigRegion* region = rgo_onig.region_new();
  rc = rgo_onig.search(regex, source, source + source_len, source,
                       source + source_len, region, 0);
  if (rc >= 0 && region != NULL && region->num_regs > 0) {
    *count = region->num_regs;
    *begins = (int*)malloc(sizeof(int) * region->num_regs);
    *ends = (int*)malloc(sizeof(int) * region->num_regs);
    memcpy(*begins, region->beg, sizeof(int) * region->num_regs);
    memcpy(*ends, region->end, sizeof(int) * region->num_regs);
  } else if (rc >= 0) {
    // Some libonig builds omit the region for a successful zero-width match.
    *count = 1;
    *begins = (int*)malloc(sizeof(int));
    *ends = (int*)malloc(sizeof(int));
    (*begins)[0] = rc;
    (*ends)[0] = rc;
  }
  if (region != NULL) rgo_onig.region_free(region, 1);
  rgo_onig.free_regex(regex);
  return rc >= 0 ? 1 : 0;
}

static int rgo_int_at(int* values, int index) { return values[index]; }
*/
import "C"

import (
	"unsafe"
)

func onigRegexpSearch(pattern, source, options string) ([]int, bool, string) {
	patternBytes := C.CBytes([]byte(pattern))
	sourceBytes := C.CBytes([]byte(source))
	defer C.free(patternBytes)
	defer C.free(sourceBytes)

	var begins *C.int
	var ends *C.int
	var count C.int
	var errorMessage *C.char
	flags := C.uint(0)
	if regexpHasBehaviorOption(options, 'i') {
		flags |= 1
	}
	if regexpHasBehaviorOption(options, 'x') {
		flags |= 2
	}
	if regexpHasBehaviorOption(options, 'm') {
		flags |= 4
	}
	rc := C.rgo_onig_search(
		(*C.uchar)(patternBytes), C.int(len(pattern)),
		(*C.uchar)(sourceBytes), C.int(len(source)), flags,
		&begins, &ends, &count, &errorMessage,
	)
	if begins != nil {
		defer C.free(unsafe.Pointer(begins))
	}
	if ends != nil {
		defer C.free(unsafe.Pointer(ends))
	}
	errText := ""
	if errorMessage != nil {
		errText = C.GoString(errorMessage)
		C.free(unsafe.Pointer(errorMessage))
	}
	if rc != 1 {
		if rc == 0 || rc == -1 {
			return nil, true, errText
		}
		return nil, false, errText
	}
	indices := make([]int, int(count)*2)
	for i := 0; i < int(count); i++ {
		indices[i*2] = int(C.rgo_int_at(begins, C.int(i)))
		indices[i*2+1] = int(C.rgo_int_at(ends, C.int(i)))
	}
	return indices, true, ""
}
