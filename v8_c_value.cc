
#include "v8_c_private.h"

extern "C" void valueWeakCallbackHandler(String id);

typedef struct
{
  String id;
} WeakCallbackParameter;

void ValueWeakCallback(const v8::WeakCallbackInfo<WeakCallbackParameter> &data)
{
  WeakCallbackParameter *param = data.GetParameter();

  valueWeakCallbackHandler(param->id);

  delete param->id.data;
  delete param;
}

extern "C"
{

  void v8_Value_SetWeak(ContextPtr pContext, ValuePtr pValue, const char *id)
  {
    VALUE_SCOPE(pContext);

    WeakCallbackParameter *param = new WeakCallbackParameter{v8_String_Create(id)};

    Value *value = static_cast<Value *>(pValue);
    value->SetWeak(param, ValueWeakCallback, v8::WeakCallbackType::kParameter);
  }

  CallResult v8_Value_Get(ContextPtr pContext, ValuePtr pObject, const char *field)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pObject)->Get(isolate);

    if (!value->IsObject())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "not an object"));
    }
    v8::MaybeLocal<v8::Object> maybeObject = value->ToObject(context);

    if (maybeObject.IsEmpty())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "empty maybe object"));
    }

    v8::Local<v8::Object> object = maybeObject.ToLocalChecked();

    v8::MaybeLocal<v8::Value>
        result = object->Get(context, v8::String::NewFromUtf8(isolate, field).ToLocalChecked());

    if (result.IsEmpty())
    {
      return v8_CallResult();
    }

    return v8_Value_ValueTuple(isolate, context, result.ToLocalChecked());
  }

  CallResult v8_Value_GetIndex(ContextPtr pContext, ValuePtr pObject, int index)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value>
        maybeObject = static_cast<Value *>(pObject)->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not an object"));
    }

    // if (maybeObject->IsTypedArray())
    // {
    //   v8::Local<v8::TypedArray> typedArray = v8::Local<v8::ArrayBuffer>::Cast(maybeObject);
    //   if (index < typedArray->ByteLength())
    //   {
    //     return v8_Value_ValueTuple(isolate, context, v8::Number::New(isolate, ((unsigned char *)typedArray->Data())[index]));
    //   }
    //   else
    //   {
    //     return v8_Value_ValueTuple(isolate, context, v8::Undefined(isolate));
    //   }
    // }
    // else
    if (maybeObject->IsArrayBuffer())
    {
      v8::Local<v8::ArrayBuffer> arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(maybeObject);
      if (index < arrayBuffer->ByteLength())
      {
        return v8_Value_ValueTuple(isolate, context, v8::Number::New(isolate, ((unsigned char *)arrayBuffer->GetBackingStore()->Data())[index]));
      }
      else
      {
        return v8_Value_ValueTuple(isolate, context, v8::Undefined(isolate));
      }
    }
    else
    {
      v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
      return v8_Value_ValueTuple(isolate, context, object->Get(context, uint32_t(index)).ToLocalChecked());
    }
  }

  int64_t v8_Object_GetInternalField(ContextPtr pContext, ValuePtr pValue, int field)
  {
    VALUE_SCOPE(pContext);

    Value *value = static_cast<Value *>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return 0;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Value> result = object->GetInternalField(field);
    v8::Maybe<int64_t> maybe = result->IntegerValue(context);

    if (maybe.IsNothing())
    {
      return 0;
    }

    return maybe.ToChecked();
  }

  Error v8_Value_Set(ContextPtr pContext, ValuePtr pValue, const char *field, ValuePtr pNewValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::MaybeLocal<v8::Object> maybeObject = value->ToObject(context);

    if (maybeObject.IsEmpty())
    {
      return v8_String_Create("Not an object");
    }

    v8::Local<v8::Object> object = maybeObject.ToLocalChecked();

    v8::Local<v8::Value> newValue = static_cast<Value *>(pNewValue)->Get(isolate);
    v8::Maybe<bool> result = object->Set(context, v8::String::NewFromUtf8(isolate, field).ToLocalChecked(), newValue);

    if (result.IsNothing())
    {
      return v8_String_Create("Something went wrong: set returned nothing.");
    }
    else if (!result.FromJust())
    {
      return v8_String_Create("Something went wrong: set failed.");
    }

    return Error{NULL, 0};
  }

  Error v8_Value_SetIndex(ContextPtr pContext, ValuePtr pValue, int index, ValuePtr pNewValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value *>(pValue)->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_String_Create("Not an object");
    }

    v8::Local<v8::Value> newValue = static_cast<Value *>(pNewValue)->Get(isolate);
    if (maybeObject->IsArrayBuffer())
    {
      v8::Local<v8::ArrayBuffer> arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(maybeObject);
      if (!newValue->IsNumber())
      {
        return v8_String_Create("Cannot assign non-number into array buffer");
      }
      else if (index >= arrayBuffer->GetBackingStore()->ByteLength())
      {
        return v8_String_Create("Cannot assign to an index beyond the size of an array buffer");
      }
      else
      {
        ((unsigned char *)arrayBuffer->GetBackingStore()->Data())[index] = newValue->ToNumber(context).ToLocalChecked()->Value();
      }
    }
    else
    {
      v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

      v8::Maybe<bool> result = object->Set(context, uint32_t(index), newValue);

      if (result.IsNothing())
      {
        return v8_String_Create("Something went wrong: set returned nothing.");
      }
      else if (!result.FromJust())
      {
        return v8_String_Create("Something went wrong: set failed.");
      }
    }

    return Error{NULL, 0};
  }

  void v8_Object_SetInternalField(ContextPtr pContext, ValuePtr pValue, int field, uint32_t newValue)
  {
    VALUE_SCOPE(pContext);

    Value *value = static_cast<Value *>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    object->SetInternalField(field, v8::Integer::New(isolate, newValue));
  }

  int v8_Object_GetInternalFieldCount(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    Value *value = static_cast<Value *>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return 0;
    }

    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    return object->InternalFieldCount();
  }

  Error v8_Value_DefineProperty(ContextPtr pContext, ValuePtr pValue, const char *key, ValuePtr pGetHandler, ValuePtr pSetHandler, bool enumerable, bool configurable)
  {
    VALUE_SCOPE(pContext);

    v8::PropertyDescriptor propertyDescriptor(static_cast<Value *>(pGetHandler)->Get(isolate), static_cast<Value *>(pSetHandler)->Get(isolate));
    propertyDescriptor.set_enumerable(enumerable);
    propertyDescriptor.set_configurable(configurable);

    Value *value = static_cast<Value *>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Maybe<bool> result = object->DefineProperty(context, v8::String::NewFromUtf8(isolate, key).ToLocalChecked(), propertyDescriptor);

    if (result.IsNothing())
    {
      return v8_String_Create("Something went wrong: define property returned nothing.");
    }
    else if (!result.FromJust())
    {
      return v8_String_Create("Something went wrong: define property failed.");
    }

    return Error{NULL, 0};
  }

  Error v8_Value_DefinePropertyValue(ContextPtr pContext, ValuePtr pValue, const char *key, ValuePtr pValueDest, bool enumerable, bool configurable, bool writable)
  {
    VALUE_SCOPE(pContext);

    v8::PropertyDescriptor propertyDescriptor(static_cast<Value *>(pValueDest)->Get(isolate), writable);
    propertyDescriptor.set_enumerable(enumerable);
    propertyDescriptor.set_configurable(configurable);

    Value *value = static_cast<Value *>(pValue);
    v8::Local<v8::Value> maybeObject = value->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Maybe<bool> result = object->DefineProperty(context, v8::String::NewFromUtf8(isolate, key).ToLocalChecked(), propertyDescriptor);

    if (result.IsNothing())
    {
      return v8_String_Create("Something went wrong: define property returned nothing.");
    }
    else if (!result.FromJust())
    {
      return v8_String_Create("Something went wrong: define property failed.");
    }

    return Error{NULL, 0};
  }

  CallResult v8_Value_GetPrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value *>(pValue)->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not an object"));
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Private> _private = static_cast<Private *>(pPrivate)->Get(isolate);

    v8::Local<v8::Value> result = object->GetPrivate(context, _private).ToLocalChecked();
    return v8_Value_ValueTuple(isolate, context, result);
  }

  Error v8_Value_SetPrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate, ValuePtr pNewValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value *>(pValue)->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();
    v8::Local<v8::Private> _private = static_cast<Private *>(pPrivate)->Get(isolate);
    v8::Local<v8::Value> newValue = static_cast<Value *>(pNewValue)->Get(isolate);

    object->SetPrivate(context, _private, newValue);

    return Error{NULL, 0};
  }

  Error v8_Value_DeletePrivate(ContextPtr pContext, ValuePtr pValue, PrivatePtr pPrivate)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> maybeObject = static_cast<Value *>(pValue)->Get(isolate);
    if (!maybeObject->IsObject())
    {
      return v8_String_Create("Not an object");
    }
    v8::Local<v8::Object> object = maybeObject->ToObject(context).ToLocalChecked();

    v8::Local<v8::Private> _private = static_cast<Private *>(pPrivate)->Get(isolate);

    object->DeletePrivate(context, _private);

    return Error{NULL, 0};
  }

  CallResult v8_Value_Call(ContextPtr pContext, ValuePtr pFunction, ValuePtr pSelf, int argc, ValuePtr *pArgv)
  {
    VALUE_SCOPE(pContext);

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    v8::Local<v8::Value> value = static_cast<Value *>(pFunction)->Get(isolate);

    if (!value->IsFunction())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "not a function"));
    }

    v8::Local<v8::Function> function = v8::Local<v8::Function>::Cast(value);

    v8::Local<v8::Value> self;
    if (pSelf == NULL)
    {
      self = context->Global();
    }
    else
    {
      self = static_cast<Value *>(pSelf)->Get(isolate);
    }

    v8::Local<v8::Value> *argv = new v8::Local<v8::Value>[argc];
    for (int i = 0; i < argc; i++)
    {
      argv[i] = static_cast<Value *>(pArgv[i])->Get(isolate);
    }

    v8::MaybeLocal<v8::Value>
        result = function->Call(context, self, argc, argv);

    delete[] argv;

    if (result.IsEmpty())
    {
      return v8_Value_ValueTuple_Exception(isolate, context, tryCatch.Exception());
    }

    return v8_Value_ValueTuple(isolate, context, result.ToLocalChecked());
  }

  CallResult v8_Value_New(ContextPtr pContext, ValuePtr pFunction, int argc, ValuePtr *pArgv)
  {
    VALUE_SCOPE(pContext);

    v8::TryCatch tryCatch(isolate);
    tryCatch.SetVerbose(false);

    v8::Local<v8::Value> value = static_cast<Value *>(pFunction)->Get(isolate);

    if (!value->IsFunction())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "not a function"));
    }
    v8::Local<v8::Function> function = v8::Local<v8::Function>::Cast(value);

    v8::Local<v8::Value> *argv = new v8::Local<v8::Value>[argc];
    for (int i = 0; i < argc; i++)
    {
      argv[i] = static_cast<Value *>(pArgv[i])->Get(isolate);
    }

    v8::MaybeLocal<v8::Object> result = function->NewInstance(context, argc, argv);

    delete[] argv;

    if (result.IsEmpty())
    {
      return v8_Value_ValueTuple_Exception(isolate, context, tryCatch.Exception());
    }

    return v8_Value_ValueTuple(isolate, context, result.ToLocalChecked());
  }

  String v8_Value_String(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    return v8_String_Create(isolate, value);
  }

  double v8_Value_Float64(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::Maybe<double> maybe = value->NumberValue(context);

    if (maybe.IsNothing())
    {
      return 0;
    }

    return maybe.ToChecked();
  }
  int64_t v8_Value_Int64(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::Maybe<int64_t> maybe = value->IntegerValue(context);

    if (maybe.IsNothing())
    {
      return 0;
    }

    return maybe.ToChecked();
  }

  bool v8_Value_Equals(ContextPtr pContext, ValuePtr pValueLeft, ValuePtr pValueRight)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> valueLeft = static_cast<Value *>(pValueLeft)->Get(isolate);
    v8::Local<v8::Value> valueRight = static_cast<Value *>(pValueRight)->Get(isolate);

    return valueLeft->Equals(context, valueRight).ToChecked();
  }

  bool v8_Value_StrictEquals(ContextPtr pContext, ValuePtr pValueLeft, ValuePtr pValueRight)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> valueLeft = static_cast<Value *>(pValueLeft)->Get(isolate);
    v8::Local<v8::Value> valueRight = static_cast<Value *>(pValueRight)->Get(isolate);

    return valueLeft->StrictEquals(valueRight);
  }

  bool v8_Value_InstanceOf(ContextPtr pContext, ValuePtr pValueLeft, ValuePtr pValueRight)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> valueLeft = static_cast<Value *>(pValueLeft)->Get(isolate);
    v8::Local<v8::Value> valueRight = static_cast<Value *>(pValueRight)->Get(isolate);

    if (!valueRight->IsObject())
    {
      return false;
    }

    v8::Local<v8::Object> objectRight = valueRight->ToObject(context).ToLocalChecked();

    v8::Maybe<bool> isInstanceOf = valueLeft->InstanceOf(context, objectRight);

    if (isInstanceOf.IsNothing())
    {
      return false;
    }

    return isInstanceOf.ToChecked();
  }

  int v8_Value_Bool(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    return value->BooleanValue(isolate) ? 1 : 0;
  }

  ByteArray v8_Value_Bytes(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);

    v8::Local<v8::ArrayBuffer> arrayBuffer;

    if (value->IsTypedArray())
    {
      arrayBuffer = v8::Local<v8::TypedArray>::Cast(value)->Buffer();
    }
    else if (value->IsArrayBuffer())
    {
      arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(value);
    }
    else if (value->IsSharedArrayBuffer())
    {
      v8::Local<v8::SharedArrayBuffer> arrayBuffer;
      arrayBuffer = v8::Local<v8::SharedArrayBuffer>::Cast(value);

      return ByteArray{
          static_cast<const char *>(arrayBuffer->GetBackingStore()->Data()),
          static_cast<int>(arrayBuffer->GetBackingStore()->ByteLength())};
    }
    else
    {
      return ByteArray{NULL, 0};
    }

    if (arrayBuffer.IsEmpty())
    {
      return ByteArray{NULL, 0};
    }

    return ByteArray{
        static_cast<const char *>(arrayBuffer->GetBackingStore()->Data()),
        static_cast<int>(arrayBuffer->GetBackingStore()->ByteLength())};
  }

  int v8_Value_ByteLength(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);

    v8::Local<v8::ArrayBuffer> arrayBuffer;

    if (value->IsTypedArray())
    {
      v8::Local<v8::TypedArray> typedArray = v8::Local<v8::TypedArray>::Cast(value);
      return static_cast<int>(typedArray->ByteLength());
    }
    else if (value->IsArrayBuffer())
    {
      arrayBuffer = v8::Local<v8::ArrayBuffer>::Cast(value);
    }
    else
    {
      return 0;
    }

    if (arrayBuffer.IsEmpty())
    {
      return 0;
    }

    return static_cast<int>(arrayBuffer->GetBackingStore()->ByteLength());
  }

  CallResult v8_Value_PromiseInfo(ContextPtr pContext, ValuePtr pValue, int *promiseState)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    if (!value->IsPromise())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "Not a promise"));
    }
    v8::Local<v8::Promise> promise = v8::Local<v8::Promise>::Cast(value);

    *promiseState = promise->State();
    if (promise->State() == v8::Promise::PromiseState::kPending)
    {
      return v8_CallResult();
    }

    v8::Local<v8::Value> result = promise->Result();
    return v8_Value_ValueTuple(isolate, context, result);
  }

  PrivatePtr v8_Private_New(IsolatePtr pIsolate, const char *name)
  {
    ISOLATE_SCOPE(static_cast<v8::Isolate *>(pIsolate));
    v8::HandleScope handleScope(isolate);

    v8::Local<v8::Private> _private = v8::Private::New(isolate, v8::String::NewFromUtf8(isolate, name).ToLocalChecked());
    return static_cast<PrivatePtr>(new Private(isolate, _private));
  }

  CallResult v8_JSON_Parse(ContextPtr pContext, const char *data)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::String> jsonString = v8_String_FromString(isolate, data);
    v8::MaybeLocal<v8::Value> maybeValue = v8::JSON::Parse(context, jsonString);

    if (maybeValue.IsEmpty())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "json parse gave an empty result"));
    }

    return v8_Value_ValueTuple(isolate, context, maybeValue.ToLocalChecked());
  }

  CallResult v8_JSON_Stringify(ContextPtr pContext, ValuePtr pValue)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::Value> value = static_cast<Value *>(pValue)->Get(isolate);
    v8::MaybeLocal<v8::String> maybeJson = v8::JSON::Stringify(context, value);

    if (maybeJson.IsEmpty())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "json stringify gave an empty result"));
    }

    return v8_Value_ValueTuple(isolate, context, maybeJson.ToLocalChecked());
  }

  ValueTuplePtr v8_Value_ValueTuple_New()
  {
    return v8_Value_ValueTuple();
  }

  CallResult v8_Value_ValueTuple_New_Error(ContextPtr pContext, const char *error)
  {
    VALUE_SCOPE(pContext);

    return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, error));
  }

  void v8_Value_ValueTuple_Retain(ValueTuplePtr vt)
  {
    vt->refCount++;
  }

  void v8_Value_ValueTuple_Release(ContextPtr pContext, ValueTuplePtr vt)
  {
    VALUE_SCOPE(pContext);

    v8_Value_ValueTuple_Release(context, vt);
  }
}

void v8_Value_ValueTuple_Release(v8::Local<v8::Context> context, ValueTuplePtr vt)
{
  if (vt->refCount == 0)
  {
    return;
  }
  if (--vt->refCount == 0)
  {
    if (vt->value)
    {
      v8::Local<v8::Value> value = static_cast<Value *>(vt->value)->Get(context->GetIsolate());

      if (value->IsObject())
      {
        v8::MaybeLocal<v8::Object> maybeObject = value->ToObject(context);

        if (!maybeObject.IsEmpty())
        {
          v8::Local<v8::Object> object = maybeObject.ToLocalChecked();
          v8::Local<v8::Private> key = ((v8::Persistent<v8::Private> *)context->GetAlignedPointerFromEmbedderData(2))->Get(context->GetIsolate());

          if (object->HasPrivate(context, key).ToChecked())
          {
            v8::Local<v8::Value> infoObjectValue = object->GetPrivate(context, key).ToLocalChecked();
            v8::MaybeLocal<v8::Object> infoMaybeObject = infoObjectValue->ToObject(context);

            if (!infoMaybeObject.IsEmpty())
            {
              v8::Local<v8::Object> infoObject = infoMaybeObject.ToLocalChecked();
              infoObject->SetAlignedPointerInInternalField(0, NULL);
            }
          }
        }
      }

      ((Value *)vt->value)->Reset();
      delete ((Value *)vt->value);
      vt->value = NULL;
      vt->kinds = kUndefined;
    }

    delete vt;
  }
}

ValueTuplePtr v8_Value_ValueTuple()
{
  ValueTuplePtr vt = new ValueTuple();
  v8_Value_ValueTuple_Retain(vt);
  return vt;
}

CallResult v8_Value_ValueTuple(v8::Isolate *isolate, v8::Local<v8::Context> context, v8::Local<v8::Value> value)
{
  v8::MaybeLocal<v8::Object> infoMaybeObject;
  v8::MaybeLocal<v8::Object> maybeObject;

  if (value->IsObject())
  {
    maybeObject = value->ToObject(context);

    if (!maybeObject.IsEmpty())
    {
      v8::Local<v8::Object> object = maybeObject.ToLocalChecked();
      v8::Local<v8::Private> key = ((v8::Persistent<v8::Private> *)context->GetAlignedPointerFromEmbedderData(2))->Get(context->GetIsolate());

      if (object->HasPrivate(context, key).ToChecked())
      {
        v8::Local<v8::Value> infoObjectValue = object->GetPrivate(context, key).ToLocalChecked();
        infoMaybeObject = infoObjectValue->ToObject(context);

        if (!infoMaybeObject.IsEmpty())
        {
          v8::Local<v8::Object> infoObject = infoMaybeObject.ToLocalChecked();
          ValueTuplePtr vt = reinterpret_cast<ValueTuplePtr>(infoObject->GetAlignedPointerFromInternalField(0));

          if (vt != NULL)
          {
            v8_Value_ValueTuple_Retain(vt);
            CallResult r = v8_CallResult();
            r.result = vt;
            return r;
          }
        }
      }
    }
  }

  ValueTuplePtr vt = v8_Value_ValueTuple();
  vt->value = new Value(isolate, value);
  vt->kinds = v8_Value_KindsFromLocal(value);

  if (value->IsObject())
  {
    if (!maybeObject.IsEmpty())
    {
      if (infoMaybeObject.IsEmpty())
      {
        v8::Local<v8::Object> object = maybeObject.ToLocalChecked();

        v8::Local<v8::Function> infoObjectConstructor = v8::Local<v8::Function>::Cast(context->GetEmbedderData(1));
        v8::Local<v8::Private> key = ((v8::Persistent<v8::Private> *)context->GetAlignedPointerFromEmbedderData(2))->Get(context->GetIsolate());

        infoMaybeObject = infoObjectConstructor->NewInstance(context);

        v8::Local<v8::Object> infoObject = infoMaybeObject.ToLocalChecked();
        object->SetPrivate(context, key, infoObject);
      }

      if (!infoMaybeObject.IsEmpty())
      {
        v8::Local<v8::Object> infoObject = infoMaybeObject.ToLocalChecked();
        infoObject->SetAlignedPointerInInternalField(0, vt);
      }
    }
  }

  CallResult r = v8_CallResult();
  r.result = vt;
  return r;
}

CallResult v8_Value_ValueTuple_Error(v8::Isolate *isolate, const v8::Local<v8::Value> &value)
{
  CallResult r = v8_CallResult();
  r.error = v8_String_Create(isolate, value);
  r.isError = true;
  return r;
}

CallResult v8_Value_ValueTuple_Exception(v8::Isolate *isolate, v8::Local<v8::Context> context, v8::Local<v8::Value> value)
{
  CallResult r = v8_Value_ValueTuple(isolate, context, value);
  r.isError = true;
  return r;
}

extern "C" CallResult v8_CallResult() {
  CallResult r = CallResult();
  memset(&r, 0, sizeof(CallResult));
  return r;
}