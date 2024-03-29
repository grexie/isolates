
#include <stdlib.h>
#include <string.h>
#include <stddef.h>
#include <stdint.h>
#include <stdbool.h>

#ifndef V8_C_BRIDGE_H
#define V8_C_BRIDGE_H

#ifdef __cplusplus
extern "C"
{
#endif

  typedef void *IsolatePtr;
  typedef void *ContextPtr;
  typedef void *ValuePtr;
  typedef void *PropertyDescriptorPtr;
  typedef void *InspectorPtr;
  typedef void *FunctionTemplatePtr;
  typedef void *ObjectTemplatePtr;
  typedef void *PrivatePtr;
  typedef void *ExternalPtr;
  typedef void *ResolverPtr;
  typedef void *ReferrerPtr;

  typedef struct
  {
    const char *data;
    int length;
  } String;

  typedef String Error;
  typedef String StartupData;
  typedef String ByteArray;

  typedef struct
  {
    size_t totalHeapSize;
    size_t totalHeapSizeExecutable;
    size_t totalPhysicalSize;
    size_t totalAvailableSize;
    size_t usedHeapSize;
    size_t heapSizeLimit;
    size_t mallocedMemory;
    size_t peakMallocedMemory;
    size_t doesZapGarbage;
  } HeapStatistics;

  typedef enum
  {
    kUndefined = 0,
    kNull,
    kName,
    kString,
    kSymbol,
    kFunction,
    kArray,
    kObject,
    kBoolean,
    kNumber,
    kExternal,
    kInt32,
    kUint32,
    kDate,
    kArgumentsObject,
    kBooleanObject,
    kNumberObject,
    kStringObject,
    kSymbolObject,
    kNativeError,
    kRegExp,
    kAsyncFunction,
    kGeneratorFunction,
    kGeneratorObject,
    kPromise,
    kMap,
    kSet,
    kMapIterator,
    kSetIterator,
    kWeakMap,
    kWeakSet,
    kArrayBuffer,
    kArrayBufferView,
    kTypedArray,
    kUint8Array,
    kUint8ClampedArray,
    kInt8Array,
    kUint16Array,
    kInt16Array,
    kUint32Array,
    kInt32Array,
    kFloat32Array,
    kFloat64Array,
    kDataView,
    kSharedArrayBuffer,
    kProxy,
    kWasmModuleObject,
    kNumKinds,
  } Kind;

  // Each kind can be represent using only single 64 bit bitmask since there
  // are less than 64 kinds so far.  If this grows beyond 64 kinds, we can switch
  // to multiple bitmasks or a dynamically-allocated array.
  typedef uint64_t Kinds;

  typedef void *Pointer;

  typedef struct
  {
    int refCount;
    ValuePtr value;
    Kinds kinds;
    Pointer internal;
  } ValueTuple;

  typedef ValueTuple *ValueTuplePtr;

  typedef struct
  {
    ValueTuplePtr result;
    Error error;
    bool isError;
  } CallResult;

  typedef struct
  {
    String funcname;
    String filename;
    int line;
    int column;
  } CallerInfo;

  typedef struct
  {
    int major, minor, build, patch;
  } Version;
  extern Version version;

  typedef enum
  {
    kFunctionCallback,
    kGetterCallback,
    kSetterCallback
  } CallbackType;

  typedef struct
  {
    CallbackType _type;
    String id;
    CallerInfo caller;
    CallResult self;
    CallResult holder;

    bool isConstructCall;
    int argc;
    CallResult *argv;

    String key;
    CallResult value;
  } CallbackInfo;

  typedef struct
  {
    String id;
    CallResult specifier;
    CallResult resourceName;
    CallResult* importAssertions;
    int importAssertionsLength;
  } ImportModuleDynamicallyCallbackInfo;

  // typedef unsigned int uint32_t;

  // v8_init must be called once before anything else.
  extern void v8_Initialize();

  extern StartupData v8_CreateSnapshotDataBlob(const char *js);

  extern IsolatePtr v8_Isolate_New(void *data, StartupData startupData);
  extern void v8_Isolate_Terminate(IsolatePtr isolate);
  extern void v8_Isolate_Release(IsolatePtr isolate);
  extern void v8_Isolate_RequestGarbageCollectionForTesting(IsolatePtr pIsolate);
  extern HeapStatistics v8_Isolate_GetHeapStatistics(IsolatePtr isolate);
  extern void v8_Isolate_LowMemoryNotification(IsolatePtr isolate);
  extern void v8_Isolate_Enter(IsolatePtr pIsolate);
  extern void v8_Isolate_Exit(IsolatePtr pIsolate);
  extern Error v8_Isolate_EnqueueMicrotask(IsolatePtr pIsolate, ContextPtr pContext, ValuePtr pFunction);
  extern void v8_Isolate_PerformMicrotaskCheckpoint(IsolatePtr pIsolate);

  extern ContextPtr v8_Context_New(IsolatePtr isolate);
  extern CallResult v8_Context_Run(ContextPtr ctx, const char *code, const char *filename, const char *id);

  extern FunctionTemplatePtr v8_FunctionTemplate_New(ContextPtr ctx, const char *id);
  extern void v8_FunctionTemplate_Release(ContextPtr ctxptr, FunctionTemplatePtr fnptr);
  extern void v8_FunctionTemplate_Inherit(ContextPtr ctxptr, FunctionTemplatePtr fnptr, FunctionTemplatePtr parentptr);
  extern void v8_FunctionTemplate_SetName(ContextPtr pContext, FunctionTemplatePtr pFunction, const char *name);
  extern CallResult v8_FunctionTemplate_GetFunction(ContextPtr ctx, FunctionTemplatePtr fn);
  extern ObjectTemplatePtr v8_FunctionTemplate_PrototypeTemplate(ContextPtr ctxptr, FunctionTemplatePtr function_ptr);
  extern ObjectTemplatePtr v8_FunctionTemplate_InstanceTemplate(ContextPtr ctxptr, FunctionTemplatePtr function_ptr);
  extern void v8_ObjectTemplate_SetAccessor(ContextPtr ctxptr, ObjectTemplatePtr object_ptr, const char *name, const char *id, bool setter);
  extern void v8_ObjectTemplate_SetInternalFieldCount(ContextPtr ctxptr, ObjectTemplatePtr object_ptr, int count);
  extern void v8_ObjectTemplate_Release(ContextPtr pContext, ObjectTemplatePtr pObjectTemplate);

  extern CallResult v8_Context_Global(ContextPtr ctx);
  extern void v8_Context_Release(ContextPtr ctx);

  typedef enum
  {
    tSTRING,
    tBOOL,
    tFLOAT64,
    tINT64,
    tOBJECT,
    tARRAY,
    tARRAYBUFFER,
    tUNDEFINED,
    tNULL,
    tDATE, // uses Float64 for msec since Unix epoch
  } ImmediateValueType;

  typedef struct
  {
    ImmediateValueType _type;
    ByteArray _data;
    bool _bool;
    double _float64;
    int64_t _int64;
  } ImmediateValue;

  extern CallResult v8_Context_Create(ContextPtr ctx, ImmediateValue val);

  extern void v8_Value_SetWeak(ContextPtr pContext, ValuePtr pValue, const char *id);
  extern CallResult v8_Value_Get(ContextPtr ctx, ValuePtr value, const char *field);
  extern Error v8_Value_Set(ContextPtr ctx, ValuePtr value,
                            const char *field, ValuePtr new_value);
  extern Error v8_Value_DefineProperty(ContextPtr ctxptr, ValuePtr valueptr, const char *key, ValuePtr getptr, ValuePtr setptr, bool enumerable, bool configurable);
  extern Error v8_Value_DefinePropertyValue(ContextPtr ctxptr, ValuePtr valueptr, const char *key, ValuePtr valuedestptr, bool enumerable, bool configurable, bool writable);
  extern CallResult v8_Value_GetIndex(ContextPtr ctx, ValuePtr value, int idx);
  extern int64_t v8_Object_GetInternalField(ContextPtr pContext, ValuePtr pValue, int field);
  extern Error v8_Value_SetIndex(ContextPtr ctx, ValuePtr value, int idx, ValuePtr new_value);
  extern void v8_Object_SetInternalField(ContextPtr ctxptr, ValuePtr value_ptr, int field, uint32_t newValue);
  extern Error v8_Value_SetPrivate(ContextPtr ctxptr, ValuePtr valueptr, PrivatePtr privateptr, ValuePtr new_valueptr);
  extern CallResult v8_Value_GetPrivate(ContextPtr ctxptr, ValuePtr valueptr, PrivatePtr privateptr);
  extern Error v8_Value_DeletePrivate(ContextPtr ctxptr, ValuePtr valueptr, PrivatePtr privateptr);
  extern int v8_Object_GetInternalFieldCount(ContextPtr pContext, ValuePtr pValue);
  extern CallResult v8_Value_Call(ContextPtr ctx, ValuePtr func, ValuePtr self, int argc, ValuePtr *argv);
  extern CallResult v8_Value_New(ContextPtr ctx,
                                 ValuePtr func,
                                 int argc, ValuePtr *argv);
  extern String v8_Value_String(ContextPtr ctx, ValuePtr value);

  extern double v8_Value_Float64(ContextPtr ctx, ValuePtr value);
  extern int64_t v8_Value_Int64(ContextPtr ctx, ValuePtr value);
  extern int v8_Value_Bool(ContextPtr ctx, ValuePtr value);
  extern bool v8_Value_Equals(ContextPtr ctx, ValuePtr left, ValuePtr right);
  extern bool v8_Value_StrictEquals(ContextPtr ctx, ValuePtr left, ValuePtr right);
  extern ByteArray v8_Value_Bytes(ContextPtr ctx, ValuePtr value);
  extern int v8_Value_ByteLength(ContextPtr ctx, ValuePtr value);
  extern bool v8_Value_InstanceOf(ContextPtr pContext, ValuePtr pValueLeft, ValuePtr pValueRight);

  extern ResolverPtr v8_Promise_NewResolver(ContextPtr pContext);
  extern Error v8_Resolver_Resolve(ContextPtr pContext, ResolverPtr pResolver, ValuePtr pValue);
  extern Error v8_Resolver_Reject(ContextPtr pContext, ResolverPtr pResolver, ValuePtr pValue);
  extern CallResult v8_Resolver_GetPromise(ContextPtr pContext, ResolverPtr pResolver);
  extern void v8_Resolver_Release(ContextPtr pContext, ResolverPtr pResolver);
  extern CallResult v8_Value_PromiseInfo(ContextPtr ctx, ValuePtr value, int *promise_state);

  extern PrivatePtr v8_Private_New(IsolatePtr isoptr, const char *name);

  extern InspectorPtr v8_Inspector_New(IsolatePtr isolate_ptr, int id);
  extern void v8_Inspector_AddContext(InspectorPtr inspector_ptr, ContextPtr ctxptr, const char *name);
  extern void v8_Inspector_RemoveContext(InspectorPtr inspector_ptr, ContextPtr ctxptr);
  extern void v8_Inspector_DispatchMessage(InspectorPtr inspector_ptr, const char *message);
  extern void v8_Inspector_Release(InspectorPtr pInspector);

  extern CallResult v8_JSON_Parse(ContextPtr pContext, const char *data);
  extern CallResult v8_JSON_Stringify(ContextPtr pContext, ValuePtr pValue);

  extern ValueTuplePtr v8_Value_ValueTuple_New();
  extern CallResult v8_Value_ValueTuple_New_Error(ContextPtr pContext, const char *error);
  extern void v8_Value_ValueTuple_Retain(ValueTuplePtr vt);
  extern void v8_Value_ValueTuple_Release(ContextPtr pContext, ValueTuplePtr vt);
  CallResult v8_CallResult();
#ifdef __cplusplus
}
#endif

#endif // !defined(V8_C_BRIDGE_H)
