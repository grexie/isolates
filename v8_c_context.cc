
#include "v8_c_private.h"

extern "C"
{
  ContextPtr v8_Context_New(IsolatePtr pIsolate)
  {
    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));
    v8::HandleScope handleScope(isolate);

    isolate->SetCaptureStackTraceForUncaughtExceptions(true);

    v8::Local<v8::ObjectTemplate> globals = v8::ObjectTemplate::New(isolate);

    v8::Local<v8::Context> context = v8::Context::New(isolate, NULL, globals);

    Context *pContext = new Context;
    pContext->pointer.Reset(isolate, context);
    pContext->isolate = isolate;

    v8::Local<v8::FunctionTemplate> infoObjectTemplate = v8::FunctionTemplate::New(isolate);
    infoObjectTemplate->InstanceTemplate()->SetInternalFieldCount(1);
    v8::Local<v8::Function> infoObjectConstructor = infoObjectTemplate->GetFunction(context).ToLocalChecked();
    context->SetEmbedderData(1, infoObjectConstructor);

    v8::Local<v8::String> key = v8::String::NewFromUtf8(isolate, "solid::info").ToLocalChecked();
    v8::Persistent<v8::Private> *privateKey = new v8::Persistent<v8::Private>(isolate, v8::Private::New(isolate, key));
    context->SetAlignedPointerInEmbedderData(2, privateKey);

    return static_cast<ContextPtr>(pContext);
  }

  CallResult v8_Context_Run(ContextPtr pContext, const char *code, const char *filename, const char *id)
  {
    VALUE_SCOPE(static_cast<Context *>(pContext));

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    filename = filename ? filename : "(no file)";

    v8::Local<v8::PrimitiveArray> hostDefinedOptions = v8::PrimitiveArray::New(isolate, 1);
    hostDefinedOptions->Set(isolate, 0, v8::String::NewFromUtf8(isolate, id).ToLocalChecked());
  
    v8::ScriptOrigin origin(
      isolate,
      v8::String::NewFromUtf8(isolate, filename).ToLocalChecked(),
      0,
      0,
      false,
      -1,
      v8::Local<v8::Value>(),
      false,
      false,
      false,
      hostDefinedOptions
    );

    v8::MaybeLocal<v8::Script> script = v8::Script::Compile(
        context,
        v8::String::NewFromUtf8(isolate, code).ToLocalChecked(),
        &origin);

    if (script.IsEmpty())
    {
      return v8_Value_ValueTuple_Exception(isolate, context, tryCatch.Exception());
    }

    v8::MaybeLocal<v8::Value> result = script.ToLocalChecked()->Run(context);

    if (result.IsEmpty())
    {
      return v8_Value_ValueTuple_Exception(isolate, context, tryCatch.Exception());
    }
    else
    {
      return v8_Value_ValueTuple(isolate, context, result.ToLocalChecked());
    }
  }

  CallResult v8_Context_Global(ContextPtr pContext)
  {
    VALUE_SCOPE(pContext);
    return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(context->Global()));
  }

  void v8_Context_Release(ContextPtr pContext)
  {
    if (pContext == NULL)
    {
      return;
    }

    Context *context = static_cast<Context *>(pContext);
    ISOLATE_SCOPE(context->isolate);
    context->pointer.Reset();
  }

  CallResult v8_Context_Create(ContextPtr pContext, ImmediateValue value)
  {
    VALUE_SCOPE(pContext);

    switch (value._type)
    {
    case tARRAY:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Array::New(isolate, value._data.length)));
    }
    case tARRAYBUFFER:
    {
      v8::Local<v8::ArrayBuffer> buffer = v8::ArrayBuffer::New(isolate, value._data.length);
      memcpy(buffer->GetBackingStore()->Data(), value._data.data, value._data.length);
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(buffer));
    }
    case tBOOL:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Boolean::New(isolate, value._bool == 1)));
    }
    case tDATE:
    {
      v8::MaybeLocal<v8::Value> date = v8::Date::New(context, value._float64);
      if (!date.IsEmpty())
      {
        return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(date.ToLocalChecked()));
      }
      break;
    }
    case tFLOAT64:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Number::New(isolate, value._float64)));
    }
    // For now, this is converted to a double on entry.
    // TODO(aroman) Consider using BigInt, but only if the V8 version supports
    // it. Check to see what V8 versions support BigInt.
    case tINT64:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Number::New(isolate, double(value._int64))));
    }
    case tOBJECT:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Object::New(isolate)));
    }
    case tSTRING:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::String::NewFromUtf8(isolate, value._data.data, v8::NewStringType::kNormal, value._data.length).ToLocalChecked()));
    }
    case tNULL:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Null(isolate)));
    }
    case tUNDEFINED:
    {
      return v8_Value_ValueTuple(isolate, context, static_cast<v8::Local<v8::Value>>(v8::Undefined(isolate)));
    }
    }

    CallResult r = v8_CallResult();

    return r;
  }
}
