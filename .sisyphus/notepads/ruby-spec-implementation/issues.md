
## MSpec Adapter Issues (Wave 0.1, Task 2)

### Ruby 4.0.1 Compatibility

**Issue**: The official ruby/spec suite's `spec_helper.rb` references `VersionGuard::FULL_RUBY_VERSION` which is not defined in Ruby 4.0.1.

**Error**:
```
NameError: uninitialized constant VersionGuard::FULL_RUBY_VERSION
/path/to/vendor/ruby/spec/spec_helper.rb:31:in '<top (required)>'
```

**Impact**: Cannot run official ruby/spec files directly with Ruby 4.0.1

**Workaround**: Created standalone spec files without the spec_helper dependency for testing

**Resolution Options**:
1. Use Ruby 3.x for running specs (recommended for now)
2. Patch spec_helper.rb to handle Ruby 4.x
3. Wait for ruby/spec to add Ruby 4.x support
4. Use standalone spec files (limits test coverage)

**Status**: Documented, workaround implemented. Not blocking current task.

