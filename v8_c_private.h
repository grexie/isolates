
#ifndef V8_C_PRIVATE_H
#define V8_C_PRIVATE_H

#include "v8_c_bridge.h"
#include <libplatform/libplatform.h>
#include <v8.h>

#define ISOLATE_SCOPE(iso)      \
  v8::Isolate *isolate = iso;   \
  v8::Locker __locker(isolate); \
  v8::Isolate::Scope __isolateScope(isolate);

#define VALUE_SCOPE(pContext)                                                             \
  ISOLATE_SCOPE(static_cast<Context *>(pContext)->isolate)                                \
  v8::HandleScope __handleScope(isolate);                                                 \
  v8::Local<v8::Context> context(static_cast<Context *>(pContext)->pointer.Get(isolate)); \
  v8::Context::Scope __contextScope(context);

#define MICROTASKS_SCOPE(isolate) \
  v8::MicrotasksScope __microtasksScope(isolate);

static v8::Platform *platform;

typedef struct
{
  v8::Persistent<v8::Context> pointer;
  v8::Isolate *isolate;
} Context;

inline String v8_String_Create(const v8::String::Utf8Value &src);
inline String v8_String_Create(v8::Isolate *isolate, const v8::Local<v8::Value> &val);
inline String v8_String_Create(const char *msg);
inline String v8_String_Create(const std::string &src);
inline std::string v8_String_ToStdString(v8::Isolate *isolate, v8::Local<v8::Value> value);
inline v8::Local<v8::String> v8_String_FromString(v8::Isolate *isolate, const String &string);

typedef v8::Persistent<v8::FunctionTemplate> FunctionTemplate;
typedef v8::Persistent<v8::ObjectTemplate> ObjectTemplate;
typedef v8::Persistent<v8::Value> Value;
typedef v8::Persistent<v8::Private> Private;
typedef v8::Persistent<v8::ScriptOrModule> Referrer;

// inline v8::Local<v8::String> v8_StackTrace_FormatException(v8::Isolate *isolate, v8::Local<v8::Context> ctx, v8::TryCatch &try_catch);
inline CallerInfo v8_StackTrace_CallerInfo(v8::Isolate *isolate);

extern "C"
{
  CallResult callbackHandler(const CallbackInfo &info);
  CallResult importModuleDynamicallyCallbackHandler(const ImportModuleDynamicallyCallbackInfo &info);
  void GetterCallbackHandler(v8::Local<v8::String> property, const v8::PropertyCallbackInfo<v8::Value> &info);
  void SetterCallbackHandler(v8::Local<v8::String> property, v8::Local<v8::Value> value, const v8::PropertyCallbackInfo<void> &info);
  void FunctionCallbackHandler(const v8::FunctionCallbackInfo<v8::Value> &args);

  void callCompletedCallback(Pointer isolate);
  void beforeCallEnteredCallback(Pointer isolate);

  void inspectorSendResponse(int inspectorId, int callId, String message);
  void inspectorSendNotification(int inspectorId, String message);
  void inspectorFlushProtocolNotifications(int inspectorId);
}

void v8_Value_ValueTuple_Release(v8::Local<v8::Context> context, ValueTuplePtr vt);
ValueTuplePtr v8_Value_ValueTuple();
CallResult v8_Value_ValueTuple(v8::Isolate *isolate, v8::Local<v8::Context> context, v8::Local<v8::Value> value);
CallResult v8_Value_ValueTuple_Error(v8::Isolate *isolate, const v8::Local<v8::Value> &value);
CallResult v8_Value_ValueTuple_Exception(v8::Isolate *isolate, v8::Local<v8::Context> context, v8::Local<v8::Value> value);

#include "v8_c_string.h"
#include "v8_c_value.h"
#include "v8_c_stack_trace.h"

#endif
