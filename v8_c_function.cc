
#include "v8_c_private.h"

extern "C"
{
  FunctionTemplatePtr v8_FunctionTemplate_New(ContextPtr pContext, const char *id)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = v8::FunctionTemplate::New(isolate, FunctionCallbackHandler, v8::String::NewFromUtf8(isolate, id).ToLocalChecked());
    return static_cast<FunctionTemplatePtr>(new FunctionTemplate(isolate, function));
  }

  void v8_FunctionTemplate_Release(ContextPtr pContext, FunctionTemplatePtr pFunction)
  {
    if (pFunction == NULL || pContext == NULL)
    {
      return;
    }

    ISOLATE_SCOPE(static_cast<Context *>(pContext)->isolate);

    FunctionTemplate *function = static_cast<FunctionTemplate *>(pFunction);
    delete function;
  }

  void v8_FunctionTemplate_Inherit(ContextPtr pContext, FunctionTemplatePtr pFunction, FunctionTemplatePtr pParentFunction)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate *>(pFunction)->Get(isolate);
    v8::Local<v8::FunctionTemplate> parentFunction = static_cast<FunctionTemplate *>(pParentFunction)->Get(isolate);
    function->Inherit(parentFunction);
  }

  void v8_FunctionTemplate_SetName(ContextPtr pContext, FunctionTemplatePtr pFunction, const char *name)
  {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate *>(pFunction)->Get(isolate);
    function->SetClassName(v8::String::NewFromUtf8(isolate, name).ToLocalChecked());
  }

  CallResult v8_FunctionTemplate_GetFunction(ContextPtr pContext, FunctionTemplatePtr pFunction)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate *>(pFunction)->Get(isolate);

    v8::MaybeLocal<v8::Function> fn = function->GetFunction(context);

    if (fn.IsEmpty())
    {
      return v8_Value_ValueTuple_Error(isolate, v8_String_FromString(isolate, "invalid function"));
    }

    return v8_Value_ValueTuple(isolate, context, fn.ToLocalChecked());
  }

  ObjectTemplatePtr v8_FunctionTemplate_PrototypeTemplate(ContextPtr pContext, FunctionTemplatePtr pFunction)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate *>(pFunction)->Get(isolate);
    return static_cast<ObjectTemplatePtr>(new ObjectTemplate(isolate, function->PrototypeTemplate()));
  }

  ObjectTemplatePtr v8_FunctionTemplate_InstanceTemplate(ContextPtr pContext, FunctionTemplatePtr pFunction)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate *>(pFunction)->Get(isolate);
    return static_cast<ObjectTemplatePtr>(new ObjectTemplate(isolate, function->InstanceTemplate()));
  }

  // void v8_ObjectTemplate_GetAccessor(ContextPtr pContext, ObjectTemplatePtr pObject, const char *name)
  // {
  //   VALUE_SCOPE(pContext);
  //   v8::Local<v8::ObjectTemplate> object = static_cast<ObjectTemplate *>(pObject)->Get(isolate);
  //   object->GetAccessor(v8::String::NewFromUtf8(isolate, name).ToLocalChecked(), (v8::AccessControl)(v8::ALL_CAN_READ | v8::ALL_CAN_WRITE), v8::PropertyAttribute::None);
  // }

  void v8_ObjectTemplate_SetAccessor(ContextPtr pContext, ObjectTemplatePtr pObject, const char *name, const char *id, bool setter)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::ObjectTemplate> object = static_cast<ObjectTemplate *>(pObject)->Get(isolate);
    object->SetAccessor(v8::String::NewFromUtf8(isolate, name).ToLocalChecked(), GetterCallbackHandler, setter ? SetterCallbackHandler : 0, v8::String::NewFromUtf8(isolate, id).ToLocalChecked(), (v8::AccessControl)(v8::ALL_CAN_READ | v8::ALL_CAN_WRITE), v8::PropertyAttribute::None);
  }

  void v8_ObjectTemplate_SetInternalFieldCount(ContextPtr pContext, ObjectTemplatePtr pObject, int count)
  {
    VALUE_SCOPE(pContext);
    v8::Local<v8::ObjectTemplate> object = static_cast<ObjectTemplate *>(pObject)->Get(isolate);
    object->SetInternalFieldCount(count);
  }

  void v8_ObjectTemplate_Release(ContextPtr pContext, ObjectTemplatePtr pObject)
  {
    if (pObject == NULL || pContext == NULL)
    {
      return;
    }

    ISOLATE_SCOPE(static_cast<Context *>(pContext)->isolate);

    ObjectTemplate *object = static_cast<ObjectTemplate *>(pObject);
    delete object;
  }

  void FunctionCallbackHandler(const v8::FunctionCallbackInfo<v8::Value> &info)
  {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    v8::Local<v8::Context> context = info.GetIsolate()->GetCurrentContext();

    String id = v8_String_Create(isolate, info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    CallResult self = v8_Value_ValueTuple(isolate, context, info.This());
    CallResult holder = v8_Value_ValueTuple(isolate, context, info.Holder());

    int argc = info.Length();
    CallResult argv[argc];
    for (int i = 0; i < argc; i++)
    {
      argv[i] = v8_Value_ValueTuple(isolate, context, info[i]);
    }

    CallResult result;
    {
      isolate->Exit();
      v8::Unlocker unlocker(isolate);

      result = callbackHandler(CallbackInfo{
          kFunctionCallback,
          id,
          callerInfo,
          self,
          holder,
          info.IsConstructCall(),
          argc,
          argv,
          String{NULL, 0},
          NULL});
    }
    isolate->Enter();

    if (result.error.data != NULL)
    {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    }
    else if (result.isError)
    {
      v8::Local<v8::Value> error = static_cast<Value *>(result.result->value)->Get(isolate);
      isolate->ThrowException(error);
    }
    else if (result.result == NULL || result.result->value == NULL)
    {
      info.GetReturnValue().Set(v8::Undefined(isolate));
    }
    else
    {
      v8::Local<v8::Value> value = static_cast<Value *>(result.result->value)->Get(isolate);
      info.GetReturnValue().Set(value);
    }

    v8_Value_ValueTuple_Release(context, result.result);
  }

  void GetterCallbackHandler(v8::Local<v8::String> property, const v8::PropertyCallbackInfo<v8::Value> &info)
  {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    v8::Local<v8::Context> context = info.GetIsolate()->GetCurrentContext();

    String id = v8_String_Create(isolate, info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    CallResult self = v8_Value_ValueTuple(isolate, context, info.This());
    CallResult holder = v8_Value_ValueTuple(isolate, context, info.Holder());
    String key = v8_String_Create(isolate, property);

    CallResult result;
    {
      isolate->Exit();
      v8::Unlocker unlocker(isolate);

      result = callbackHandler(CallbackInfo{
          kGetterCallback,
          id,
          callerInfo,
          self,
          holder,
          false,
          0,
          NULL,
          key,
          NULL});
    }
    isolate->Enter();

    if (result.error.data != NULL)
    {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    }
    else if (result.isError)
    {
      v8::Local<v8::Value> error = static_cast<Value *>(result.result->value)->Get(isolate);
      isolate->ThrowException(error);
    }
    else if (result.result == NULL || result.result->value == NULL)
    {
      info.GetReturnValue().Set(v8::Undefined(isolate));
    }
    else
    {
      v8::Local<v8::Value> value = static_cast<Value *>(result.result->value)->Get(isolate);
      info.GetReturnValue().Set(value);
    }

    v8_Value_ValueTuple_Release(context, result.result);
  }

  void SetterCallbackHandler(v8::Local<v8::String> property, v8::Local<v8::Value> value, const v8::PropertyCallbackInfo<void> &info)
  {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    v8::Local<v8::Context> context = info.GetIsolate()->GetCurrentContext();

    String id = v8_String_Create(isolate, info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    CallResult self = v8_Value_ValueTuple(isolate, context, info.This());
    CallResult holder = v8_Value_ValueTuple(isolate, context, info.Holder());
    String key = v8_String_Create(isolate, property);
    CallResult valueTuple = v8_Value_ValueTuple(isolate, context, value);

    CallResult result;
    {
      isolate->Exit();
      v8::Unlocker unlocker(isolate);

      result = callbackHandler(CallbackInfo{
          kSetterCallback,
          id,
          callerInfo,
          self,
          holder,
          false,
          0,
          NULL,
          key,
          valueTuple});
    }
    isolate->Enter();

    if (result.error.data != NULL)
    {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    }
    else if (result.isError)
    {
      v8::Local<v8::Value> error = static_cast<Value *>(result.result->value)->Get(isolate);
      isolate->ThrowException(error);
    }

    v8_Value_ValueTuple_Release(context, result.result);
  }
}
