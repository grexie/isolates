
#include "v8_c_private.h"

extern "C" {
  ContextPtr v8_Context_New(IsolatePtr pIsolate) {
    ISOLATE_SCOPE(static_cast<v8::Isolate*>(pIsolate));
    v8::HandleScope handleScope(isolate);

    isolate->SetCaptureStackTraceForUncaughtExceptions(true);

    v8::Local<v8::ObjectTemplate> globals = v8::ObjectTemplate::New(isolate);

    Context* context = new Context;
    context->pointer.Reset(isolate, v8::Context::New(isolate, NULL, globals));
    context->isolate = isolate;
    return static_cast<ContextPtr>(context);
  }

  ValueTuple v8_Context_Run(ContextPtr pContext, const char* code, const char* filename) {
    VALUE_SCOPE(static_cast<Context*>(pContext));

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    filename = filename ? filename : "(no file)";

    v8::Local<v8::Script> script = v8::Script::Compile(
      v8::String::NewFromUtf8(isolate, code),
      v8::String::NewFromUtf8(isolate, filename)
    );

    if (script.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_StackTrace_FormatException(isolate, context, tryCatch));
    }

    v8::Local<v8::Value> result = script->Run();

    if (result.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_StackTrace_FormatException(isolate, context, tryCatch));
    } else {
      return v8_Value_ValueTuple(isolate, result);
    }
  }

  ValuePtr v8_Context_Global(ContextPtr pContext) {
    VALUE_SCOPE(pContext);
    return new Value(isolate, context->Global());
  }

  void v8_Context_Release(ContextPtr pContext) {
    if (pContext == NULL) {
      return;
    }

    Context* context = static_cast<Context*>(pContext);
    ISOLATE_SCOPE(context->isolate);
    context->pointer.Reset();
  }

  ValuePtr v8_Context_Create(ContextPtr pContext, ImmediateValue value) {
    VALUE_SCOPE(pContext);

    switch (value._type) {
    case tARRAY: {
      return new Value(isolate, v8::Array::New(isolate, value._data.length));
    }
    case tARRAYBUFFER: {
      v8::Local<v8::ArrayBuffer> buffer = v8::ArrayBuffer::New(isolate, value._data.length);
      memcpy(buffer->GetContents().Data(), value._data.data, value._data.length);
      return new Value(isolate, buffer);
    }
    case tBOOL: {
      return new Value(isolate, v8::Boolean::New(isolate, value._bool == 1));
    }
    case tDATE: {
      return new Value(isolate, v8::Date::New(isolate, value._float64));
    }
    case tFLOAT64: {
      return new Value(isolate, v8::Number::New(isolate, value._float64));
    }
    // For now, this is converted to a double on entry.
    // TODO(aroman) Consider using BigInt, but only if the V8 version supports
    // it. Check to see what V8 versions support BigInt.
    case tINT64: {
      return new Value(isolate, v8::Number::New(isolate, double(value._int64)));
    }
    case tOBJECT: {
      return new Value(isolate, v8::Object::New(isolate));
    }
    case tSTRING: {
      return new Value(isolate, v8::String::NewFromUtf8(isolate, value._data.data, v8::NewStringType::kNormal, value._data.length).ToLocalChecked());
    }
    case tUNDEFINED: {
      return new Value(isolate, v8::Undefined(isolate));
    }
    }

    return NULL;
  }
}
