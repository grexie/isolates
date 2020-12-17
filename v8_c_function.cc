
#include "v8_c_private.h"

extern "C" {
  FunctionTemplatePtr v8_FunctionTemplate_New(ContextPtr pContext, const char* id) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = v8::FunctionTemplate::New(isolate, FunctionCallbackHandler, v8::String::NewFromUtf8(isolate, id));
    return static_cast<FunctionTemplatePtr>(new FunctionTemplate(isolate, function));
  }

  void v8_FunctionTemplate_Release(ContextPtr pContext, FunctionTemplatePtr pFunction) {
    if (pFunction == NULL || pContext == NULL)  {
      return;
    }

    ISOLATE_SCOPE(static_cast<Context*>(pContext)->isolate);

    FunctionTemplate* function = static_cast<FunctionTemplate*>(pFunction);
    delete function;
  }

  void v8_FunctionTemplate_Inherit(ContextPtr pContext, FunctionTemplatePtr pFunction, FunctionTemplatePtr pParentFunction) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    v8::Local<v8::FunctionTemplate> parentFunction = static_cast<FunctionTemplate*>(pParentFunction)->Get(isolate);
    function->Inherit(parentFunction);
  }

  void v8_FunctionTemplate_SetName(ContextPtr pContext, FunctionTemplatePtr pFunction, const char* name) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    function->SetClassName(v8::String::NewFromUtf8(isolate, name));
  }

  void v8_FunctionTemplate_SetHiddenPrototype(ContextPtr pContext, FunctionTemplatePtr pFunction, bool value) {
    VALUE_SCOPE(pContext);

    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    function->SetHiddenPrototype(value);
  }

  ValuePtr v8_FunctionTemplate_GetFunction(ContextPtr pContext, FunctionTemplatePtr pFunction) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    return new Value(isolate, function->GetFunction());
  }

  ObjectTemplatePtr v8_FunctionTemplate_PrototypeTemplate(ContextPtr pContext, FunctionTemplatePtr pFunction) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    return static_cast<ObjectTemplatePtr>(new ObjectTemplate(isolate, function->PrototypeTemplate()));
  }

  ObjectTemplatePtr v8_FunctionTemplate_InstanceTemplate(ContextPtr pContext, FunctionTemplatePtr pFunction) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::FunctionTemplate> function = static_cast<FunctionTemplate*>(pFunction)->Get(isolate);
    return static_cast<ObjectTemplatePtr>(new ObjectTemplate(isolate, function->InstanceTemplate()));
  }

  void v8_ObjectTemplate_SetAccessor(ContextPtr pContext, ObjectTemplatePtr pObject, const char* name, const char* id, bool setter) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::ObjectTemplate> object = static_cast<ObjectTemplate*>(pObject)->Get(isolate);
    object->SetAccessor(v8::String::NewFromUtf8(isolate, name), GetterCallbackHandler, setter ? SetterCallbackHandler : 0, v8::String::NewFromUtf8(isolate, id), (v8::AccessControl)(v8::ALL_CAN_READ | v8::ALL_CAN_WRITE), v8::PropertyAttribute::None);
  }

  void v8_ObjectTemplate_SetInternalFieldCount(ContextPtr pContext, ObjectTemplatePtr pObject, int count) {
    VALUE_SCOPE(pContext);
    v8::Local<v8::ObjectTemplate> object = static_cast<ObjectTemplate*>(pObject)->Get(isolate);
    object->SetInternalFieldCount(count);
  }

  void v8_ObjectTemplate_Release(ContextPtr pContext, ObjectTemplatePtr pObject) {
    if (pObject == NULL || pContext == NULL)  {
      return;
    }

    ISOLATE_SCOPE(static_cast<Context*>(pContext)->isolate);

    ObjectTemplate* object = static_cast<ObjectTemplate*>(pObject);
    delete object;
  }

  void FunctionCallbackHandler(const v8::FunctionCallbackInfo<v8::Value>& info) {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    String id = v8_String_Create(info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    ValueTuple self = v8_Value_ValueTuple(isolate, info.This());
    ValueTuple holder = v8_Value_ValueTuple(isolate, info.Holder());

    int argc = info.Length();
    ValueTuple argv[argc];
    for (int i = 0; i < argc; i++) {
      argv[i] = v8_Value_ValueTuple(isolate, info[i]);
    }

    ValueTuple result;
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
        NULL
      });
    }
    isolate->Enter();

    if (result.error.data != NULL) {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    } else if (result.value == NULL) {
      info.GetReturnValue().Set(v8::Undefined(isolate));
    } else {
      info.GetReturnValue().Set(*static_cast<Value*>(result.value));
    }
  }

  void GetterCallbackHandler(v8::Local<v8::String> property, const v8::PropertyCallbackInfo<v8::Value>& info) {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    String id = v8_String_Create(isolate, info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    ValueTuple self = v8_Value_ValueTuple(isolate, info.This());
    ValueTuple holder = v8_Value_ValueTuple(isolate, info.Holder());
    String key = v8_String_Create(isolate, property);

    ValueTuple result;
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
        NULL
      });
    }
    isolate->Enter();

    if (result.error.data != NULL) {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    } else if (result.value == NULL) {
      info.GetReturnValue().Set(v8::Undefined(isolate));
    } else {
      info.GetReturnValue().Set(*static_cast<Value*>(result.value));
    }
  }

  void SetterCallbackHandler(v8::Local<v8::String> property, v8::Local<v8::Value> value, const v8::PropertyCallbackInfo<void>& info) {
    ISOLATE_SCOPE(info.GetIsolate());
    v8::HandleScope handleScope(isolate);

    String id = v8_String_Create(isolate, info.Data());
    CallerInfo callerInfo = v8_StackTrace_CallerInfo(isolate);
    ValueTuple self = v8_Value_ValueTuple(isolate, info.This());
    ValueTuple holder = v8_Value_ValueTuple(isolate, info.Holder());
    String key = v8_String_Create(isolate, property);
    ValueTuple valueTuple = v8_Value_ValueTuple(isolate, value);

    ValueTuple result;
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
        valueTuple
      });
    }
    isolate->Enter();

    if (result.error.data != NULL) {
      v8::Local<v8::Value> error = v8::Exception::Error(v8_String_FromString(isolate, result.error));
      isolate->ThrowException(error);
    }
  }
}
