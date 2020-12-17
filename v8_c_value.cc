
#include "v8_c_private.h"

extern "C" void valueWeakCallbackHandler(String id);

typedef struct {
  String id;
} WeakCallbackParameter;

void ValueWeakCallback(const v8::WeakCallbackInfo<WeakCallbackParameter>& data) {
  WeakCallbackParameter* param = data.GetParameter();

  valueWeakCallbackHandler(param->id);

  delete param->id.data;
  delete param;
}

extern "C" {

  void v8_Value_SetWeak(ContextPtr pContext, ValuePtr pValue, const char* id) {
    VALUE_SCOPE(pContext);

    WeakCallbackParameter* param = new WeakCallbackParameter{v8_String_Create(id)};

    Value* value = static_cast<Value*>(pValue);
    value->SetWeak(param, ValueWeakCallback, v8::WeakCallbackType::kParameter);
  }

  ValueTuple v8_Value_Get(ContextPtr pContext, ValuePtr pObject, const char* field) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pObject)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not an object"));
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Local<v8::Value> value = object->Get(context, v8::String::NewFromUtf8(isolate, field)).ToLocalChecked();
    return v8_Value_ValueTuple(isolate, value);
  }

  ValueTuple v8_Value_GetIndex(ContextPtr pContext, ValuePtr pObject, int index) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pObject)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not an object"));
    }

    if (maybeObject->IsArrayBuffer()) {
      v8::Local<v8::ArrayBuffer> arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(maybeObject);
      if (index < arrayBuffer->GetContents().ByteLength()) {
        return v8_Value_ValueTuple(isolate, v8::Number::New(isolate, ((unsigned char*)arrayBuffer->GetContents().Data())[index]));
      } else {
        return v8_Value_ValueTuple(isolate, v8::Undefined(isolate));
      }
    } else {
      v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
      return v8_Value_ValueTuple(isolate, object->Get(context, uint32_t(index)).ToLocalChecked());
    }
  }

  int64_t v8_Object_GetInternalField(ContextPtr pContext, ValuePtr pValue, int field) {
    VALUE_SCOPE(pContext);

    Value* value = static_cast<Value*>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject()) {
      return 0;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Value> result = object->GetInternalField(field);
    v8::Maybe<int64_t> maybe = result->IntegerValue(context);

    if (maybe.IsNothing()) {
      return 0;
    }

    return maybe.ToChecked();
  }

  Error v8_Value_Set(ContextPtr pContext, ValuePtr pValue, const char* field, ValuePtr pNewValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pValue)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Local<v8::Value> newValue = static_cast<Value*>(pNewValue)->Get(isolate);
    v8::Maybe<bool> result = object->Set(context, v8::String::NewFromUtf8(isolate, field), newValue);

    if (result.IsNothing()) {
      return v8_String_Create("Something went wrong: set returned nothing.");
    } else if (!result.FromJust()) {
      return v8_String_Create("Something went wrong: set failed.");
    }

    return Error{ NULL, 0 };
  }

  Error v8_Value_SetIndex(ContextPtr pContext, ValuePtr pValue, int index, ValuePtr pNewValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pValue)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_String_Create("Not an object");
    }

    v8::Local<v8::Value> newValue = static_cast<Value*>(pNewValue)->Get(isolate);
    if (maybeObject->IsArrayBuffer()) {
      v8::Local<v8::ArrayBuffer> arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(maybeObject);
      if (!newValue->IsNumber()) {
        return v8_String_Create("Cannot assign non-number into array buffer");
      } else if (index >= arrayBuffer->GetContents().ByteLength()) {
        return v8_String_Create("Cannot assign to an index beyond the size of an array buffer");
      } else {
        ((unsigned char*)arrayBuffer->GetContents().Data())[index] = newValue->ToNumber(context).ToLocalChecked()->Value();
      }
    } else {
      v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

      v8::Maybe<bool> result = object->Set(context, uint32_t(index), newValue);

      if (result.IsNothing()) {
        return v8_String_Create("Something went wrong: set returned nothing.");
      } else if (!result.FromJust()) {
        return v8_String_Create("Something went wrong: set failed.");
      }
    }

    return Error{NULL, 0};
  }

  void v8_Object_SetInternalField(ContextPtr pContext, ValuePtr pValue, int field, uint32_t newValue) {
    VALUE_SCOPE(pContext);

    Value* value = static_cast<Value*>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject()) {
      return;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    object->SetInternalField(field, v8::Integer::New(isolate, newValue));
  }

  int v8_Object_GetInternalFieldCount(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    Value* value = static_cast<Value*>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject()) {
      return 0;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    return object->InternalFieldCount();
  }

  Error v8_Value_DefineProperty(ContextPtr pContext, ValuePtr pValue, const char* key, ValuePtr pGetHandler, ValuePtr pSetHandler, bool enumerable, bool configurable) {
    VALUE_SCOPE(pContext);

    v8::PropertyDescriptor propertyDescriptor(static_cast<Value*>(pGetHandler)->Get(isolate), static_cast<Value*>(pSetHandler)->Get(isolate));
    propertyDescriptor.set_enumerable(enumerable);
    propertyDescriptor.set_configurable(configurable);

    Value* value = static_cast<Value*>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Maybe<bool> result = object->DefineProperty(context, v8::String::NewFromUtf8(isolate, key), propertyDescriptor);

    if (result.IsNothing()) {
      return v8_String_Create("Something went wrong: define property returned nothing.");
    } else if (!result.FromJust()) {
      return v8_String_Create("Something went wrong: define property failed.");
    }

    return Error{NULL, 0};
  }

  ValueTuple v8_Value_GetPrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pValue)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not an object"));
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Private> _private = static_cast<Private*>(pPrivate)->Get(isolate);

    v8::Local<v8::Value> result = object->GetPrivate(context, _private).ToLocalChecked();
    return v8_Value_ValueTuple(isolate, result);
  }


  Error v8_Value_SetPrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate, ValuePtr pNewValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pValue)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Private> _private = static_cast<Private*>(pPrivate)->Get(isolate);
    v8::Local<v8::Value> newValue = static_cast<Value*>(pNewValue)->Get(isolate);

    object->SetPrivate(context, _private, newValue);

    return Error{NULL, 0};
  }



  Error v8_Value_DeletePrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value*>(pValue)->Get(isolate);
    if (!maybeObject->IsObject()) {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Local<v8::Private> _private = static_cast<Private*>(pPrivate)->Get(isolate);

    object->DeletePrivate(context, _private);

    return Error{NULL, 0};
  }

  ValueTuple v8_Value_Call(ContextPtr pContext, ValuePtr pFunction, ValuePtr pSelf, int argc, ValuePtr* pArgv) {
    VALUE_SCOPE(pContext);

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    v8::Local<v8::Value> value = static_cast<Value*>(pFunction)->Get(isolate);
    if (!value->IsFunction()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not a function"));
    }
    v8::Local<v8::Function> function = v8::Local<v8::Function>::Cast(value);

    v8::Local<v8::Value> self;
    if (pSelf == NULL) {
      self = context->Global();
    } else {
      self = static_cast<Value*>(pSelf)->Get(isolate);
    }

    v8::Local<v8::Value>* argv = new v8::Local<v8::Value>[argc];
    for (int i = 0; i < argc; i++) {
      argv[i] = static_cast<Value*>(pArgv[i])->Get(isolate);
    }

    v8::MaybeLocal<v8::Value> result = function->Call(context, self, argc, argv);

    delete[] argv;

    if (result.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_StackTrace_FormatException(isolate, context, tryCatch));
    }

    return v8_Value_ValueTuple(isolate, result.ToLocalChecked());
  }

  ValueTuple v8_Value_New(ContextPtr pContext, ValuePtr pFunction, int argc, ValuePtr* pArgv) {
    VALUE_SCOPE(pContext);

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    v8::Local<v8::Value> value = static_cast<Value*>(pFunction)->Get(isolate);
    if (!value->IsFunction()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not a function"));
    }
    v8::Local<v8::Function> function = v8::Local<v8::Function>::Cast(value);

    v8::Local<v8::Value>* argv = new v8::Local<v8::Value>[argc];
    for (int i = 0; i < argc; i++) {
      argv[i] = static_cast<Value*>(pArgv[i])->Get(isolate);
    }

    v8::MaybeLocal<v8::Object> result = function->NewInstance(context, argc, argv);

    delete[] argv;

    if (result.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_StackTrace_FormatException(isolate, context, tryCatch));
    }

    return v8_Value_ValueTuple(isolate, result.ToLocalChecked());
  }

  void v8_Value_Release(ContextPtr pContext, ValuePtr pValue) {
    if (pValue == NULL || pContext == NULL)  {
      return;
    }

    ISOLATE_SCOPE(static_cast<Context*>(pContext)->isolate);

    Value* value = static_cast<Value*>(pValue);
    value->Reset();
    delete value;
  }

  String v8_Value_String(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    return v8_String_Create(value);
  }

  double v8_Value_Float64(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    v8::Maybe<double> maybe = value->NumberValue(context);

    if (maybe.IsNothing()) {
      return 0;
    }

    return maybe.ToChecked();
  }
  int64_t v8_Value_Int64(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    v8::Maybe<int64_t> maybe = value->IntegerValue(context);

    if (maybe.IsNothing()) {
      return 0;
    }

    return maybe.ToChecked();
  }

  int v8_Value_Bool(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    v8::Maybe<bool> maybe = value->BooleanValue(context);

    if (maybe.IsNothing()) {
      return 0;
    }

    return maybe.ToChecked() ? 1 : 0;
  }

  ByteArray v8_Value_Bytes(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);

    v8::Local<v8::ArrayBuffer> arrayBuffer;

    if (value->IsTypedArray()) {
      arrayBuffer = v8::Local<v8::TypedArray>::Cast(value)->Buffer();
    } else if (value->IsArrayBuffer()) {
      arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(value);
    } else {
      return ByteArray{ NULL, 0 };
    }

    if (arrayBuffer.IsEmpty()) {
      return ByteArray{ NULL, 0 };
    }

    return ByteArray{
      static_cast<const char*>(arrayBuffer->GetContents().Data()),
      static_cast<int>(arrayBuffer->GetContents().ByteLength())
    };
  }

  ValueTuple v8_Value_PromiseInfo(ContextPtr pContext, ValuePtr pValue, int* promiseState) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    if (!value->IsPromise()) {
      return v8_Value_ValueTuple_Error(isolate v8_String_FromString(isolate, "Not a promise"));
    }
    v8::Local<v8::Promise> promise = v8::Local<v8::Promise>::Cast(value);

    *promiseState = promise->State();
    if (promise->State() == v8::Promise::PromiseState::kPending) {
      return v8_Value_ValueTuple();
    }

    v8::Local<v8::Value> result = promise->Result();
    return v8_Value_ValueTuple(isolate, result);
  }

  PrivatePtr v8_Private_New(IsolatePtr pIsolate, const char *name) {
    ISOLATE_SCOPE(static_cast<v8::Isolate*>(pIsolate));
    v8::HandleScope handleScope(isolate);

    v8::Local<v8::Private> _private = v8::Private::New(isolate, v8::String::NewFromUtf8(isolate, name));
    return static_cast<PrivatePtr>(new Private(isolate, _private));
  }

  ValueTuple v8_JSON_Parse(ContextPtr pContext, const char* data) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::String> jsonString = v8_String_FromString(isolate, data);
    v8::MaybeLocal<v8::Value> maybeValue = v8::JSON::Parse(context, jsonString);

    if (maybeValue.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "json parse gave an empty result"));
    }

    return v8_Value_ValueTuple(isolate, maybeValue.ToLocalChecked());
  }

  ValueTuple v8_JSON_Stringify(ContextPtr pContext, ValuePtr pValue) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value*>(pValue)->Get(isolate);
    v8::MaybeLocal<v8::String> maybeJson = v8::JSON::Stringify(context, value);

    if (maybeJson.IsEmpty()) {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "json stringify gave an empty result"));
    }

    return v8_Value_ValueTuple(isolate, maybeJson.ToLocalChecked());
  }

}
