package core

import "github.com/GoLangDream/rgo/pkg/object"

// installForwardableModule provides the executable part of Ruby's
// Forwardable stdlib without evaluating its version-dependent dynamic heredoc
// method generator. The public delegation API is ordinary Ruby source, so
// generated methods still use define_method and normal send semantics; only
// the stdlib bootstrap is supplied by RGo.
func installForwardableModule(objectClass *object.Class) {
	if objectClass == nil || EvalSource == nil {
		return
	}
	if existing := objectClass.Constants["Forwardable"]; existing != nil && existing.Type == object.ValueModule {
		return
	}
	// The VM has a strict fast path for the wrapper shape emitted by
	// Forwardable.  EvalSource normally inherits the requiring file (for
	// example Prawn's internals.rb), which would make these generated methods
	// indistinguishable from ordinary user define_method bodies.  Give only
	// this compatibility bootstrap the stdlib identity expected by that guard;
	// restore the caller's path immediately so __FILE__/backtraces outside the
	// generated wrappers remain unchanged.
	previousPath, previousAbsolutePath := CurrentSpecFile, CurrentSpecFileAbsolute
	CurrentSpecFile, CurrentSpecFileAbsolute = "/forwardable.rb", "/forwardable.rb"
	result := EvalSource(`module Forwardable
  def instance_delegate(hash)
    hash.each do |methods, accessor|
      names = methods.respond_to?(:each) ? methods : [methods]
      names.each do |name|
        if accessor.to_s.start_with?("@")
          define_method(name) do |*args, &block|
            target = instance_variable_get(accessor)
            target.__send__(name, *args, &block)
          end
        else
          define_method(name) do |*args, &block|
            target = send(accessor)
            target.__send__(name, *args, &block)
          end
        end
      end
    end
  end

  alias delegate instance_delegate

  def def_instance_delegators(accessor, *methods)
    methods.each do |method|
      def_instance_delegator(accessor, method)
    end
  end

  def def_instance_delegator(accessor, method, ali = method)
    if accessor.to_s.start_with?("@")
      define_method(ali) do |*args, &block|
        target = instance_variable_get(accessor)
        target.__send__(method, *args, &block)
      end
    else
      define_method(ali) do |*args, &block|
        target = send(accessor)
        target.__send__(method, *args, &block)
      end
    end
  end

  alias def_delegators def_instance_delegators
  alias def_delegator def_instance_delegator
end

module SingleForwardable
  def single_delegate(hash)
    hash.each do |methods, accessor|
      names = methods.respond_to?(:each) ? methods : [methods]
      names.each do |name|
        def_single_delegator(accessor, name)
      end
    end
  end

  alias delegate single_delegate

  def def_single_delegators(accessor, *methods)
    methods.each do |method|
      def_single_delegator(accessor, method)
    end
  end

  def def_single_delegator(accessor, method, ali = method)
    if accessor.to_s.start_with?("@")
      singleton_class.define_method(ali) do |*args, &block|
        target = instance_variable_get(accessor)
        target.__send__(method, *args, &block)
      end
    else
      singleton_class.define_method(ali) do |*args, &block|
        target = send(accessor)
        target.__send__(method, *args, &block)
      end
    end
  end

  alias def_delegators def_single_delegators
  alias def_delegator def_single_delegator
end`)
	CurrentSpecFile, CurrentSpecFileAbsolute = previousPath, previousAbsolutePath
	if result != nil && result.Type == object.ValueException {
		LastException = result
	}
}
