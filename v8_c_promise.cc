
#include "v8_c_private.h"

typedef v8::Persistent<v8::Promise::Resolver> Resolver;

extern "C"
{

  ResolverPtr v8_Promise_NewResolver(ContextPtr pContext)
  {
    VALUE_SCOPE(pContext);

    v8::MaybeLocal<v8::Promise::Resolver> resolver = v8::Promise::Resolver::New(context);
    if (resolver.IsEmpty())
    {
      return NULL;
    }

    return new Resolver(isolate, resolver.ToLocalChecked());
  }

  Error v8_Resolver_Resolve(ContextPtr pContext, ResolverPtr pResolver, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::Local<v8::Promise::Resolver> resolver = static_cast<Resolver *>(pResolver)->Get(isolate);

    v8::Maybe<bool> result = resolver->Resolve(context, value);

    if (result.IsNothing())
    {
      return v8_String_Create("Something went wrong: resolve returned nothing.");
    }
    else if (!result.FromJust())
    {
      return v8_String_Create("Something went wrong: resolve failed.");
    }

    return Error{NULL, 0};
  }

  Error v8_Resolver_Reject(ContextPtr pContext, ResolverPtr pResolver, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::Local<v8::Promise::Resolver> resolver = static_cast<Resolver *>(pResolver)->Get(isolate);

    v8::Maybe<bool> result = resolver->Reject(context, value);

    if (result.IsNothing())
    {
      return v8_String_Create("Something went wrong: resolve returned nothing.");
    }
    else if (!result.FromJust())
    {
      return v8_String_Create("Something went wrong: resolve failed.");
    }

    return Error{NULL, 0};
  }

  ValuePtr v8_Resolver_GetPromise(ContextPtr pContext, ResolverPtr pResolver)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Promise::Resolver> resolver = static_cast<Resolver *>(pResolver)->Get(isolate);
    return new Value(isolate, resolver->GetPromise());
  }

  void v8_Resolver_Release(ContextPtr pContext, ResolverPtr pResolver)
  {
    VALUE_SCOPE(pContext);

    Resolver *resolver = static_cast<Resolver *>(pResolver);
    resolver->Reset();
    delete resolver;
  }
}
