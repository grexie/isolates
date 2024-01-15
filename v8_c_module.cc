
#include "v8_c_private.h"

v8::MaybeLocal<v8::Promise> ImportModuleDynamicallyCallbackHandler(v8::Local<v8::Context> context, v8::Local<v8::Data> hostDefinedOptions, v8::Local<v8::Value> resourceName, v8::Local<v8::String> specifier, v8::Local<v8::FixedArray> importAssertions)
{
  ISOLATE_SCOPE(context->GetIsolate());
  v8::HandleScope handleScope(isolate);

  CallResult rResourceName = v8_Value_ValueTuple(isolate, context, resourceName);
  CallResult rSpecifier = v8_Value_ValueTuple(isolate, context, specifier);

  v8::Local<v8::PrimitiveArray> hostDefinedOptionsArray = v8::Local<v8::PrimitiveArray>::Cast(hostDefinedOptions);
  String id = v8_String_Create(isolate, hostDefinedOptionsArray->Get(isolate, 0));

  int rImportAssertionsLength = importAssertions->Length();
  CallResult rImportAssertions[rImportAssertionsLength];
  for (int i = 0; i < rImportAssertionsLength; i++)
  {
    v8::Local<v8::Data> importAssertion = importAssertions->Get(context, i);
    rImportAssertions[i] = v8_Value_ValueTuple(isolate, context, v8::Local<v8::Value>::Cast(importAssertion));
  }

  CallResult result;
  {
    isolate->Exit();
    v8::Unlocker unlocker(isolate);

    result = importModuleDynamicallyCallbackHandler(ImportModuleDynamicallyCallbackInfo{
      id,
      rSpecifier,
      rResourceName,
      rImportAssertions,
      rImportAssertionsLength,
    });
  }
  isolate->Enter();

  v8::Local<v8::Value> value = static_cast<Value *>(result.result->value)->Get(isolate);
  v8_Value_ValueTuple_Release(context, result.result);

  return v8::Local<v8::Promise>::Cast(value);
}

void v8_Isolate_AddImportModuleDynamicallyCallbackHandler(IsolatePtr pIsolate) {
  v8::Isolate *isolate = static_cast<v8::Isolate *>(pIsolate);
  isolate->SetHostImportModuleDynamicallyCallback(ImportModuleDynamicallyCallbackHandler);
}