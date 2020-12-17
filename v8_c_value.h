
#ifndef V8_C_VALUE_H
#define V8_C_VALUE_H

inline Kinds v8_Value_KindsFromLocal(v8::Local<v8::Value> value) {
  Kinds kinds = 0;

  if (value->IsUndefined())         kinds |= (1ULL << Kind::kUndefined        );
  if (value->IsNull())              kinds |= (1ULL << Kind::kNull             );
  if (value->IsName())              kinds |= (1ULL << Kind::kName             );
  if (value->IsString())            kinds |= (1ULL << Kind::kString           );
  if (value->IsSymbol())            kinds |= (1ULL << Kind::kSymbol           );
  if (value->IsObject())            kinds |= (1ULL << Kind::kObject           );
  if (value->IsArray())             kinds |= (1ULL << Kind::kArray            );
  if (value->IsBoolean())           kinds |= (1ULL << Kind::kBoolean          );
  if (value->IsNumber())            kinds |= (1ULL << Kind::kNumber           );
  if (value->IsExternal())          kinds |= (1ULL << Kind::kExternal         );
  if (value->IsInt32())             kinds |= (1ULL << Kind::kInt32            );
  if (value->IsUint32())            kinds |= (1ULL << Kind::kUint32           );
  if (value->IsDate())              kinds |= (1ULL << Kind::kDate             );
  if (value->IsArgumentsObject())   kinds |= (1ULL << Kind::kArgumentsObject  );
  if (value->IsBooleanObject())     kinds |= (1ULL << Kind::kBooleanObject    );
  if (value->IsNumberObject())      kinds |= (1ULL << Kind::kNumberObject     );
  if (value->IsStringObject())      kinds |= (1ULL << Kind::kStringObject     );
  if (value->IsSymbolObject())      kinds |= (1ULL << Kind::kSymbolObject     );
  if (value->IsNativeError())       kinds |= (1ULL << Kind::kNativeError      );
  if (value->IsRegExp())            kinds |= (1ULL << Kind::kRegExp           );
  if (value->IsFunction())          kinds |= (1ULL << Kind::kFunction         );
  if (value->IsAsyncFunction())     kinds |= (1ULL << Kind::kAsyncFunction    );
  if (value->IsGeneratorFunction()) kinds |= (1ULL << Kind::kGeneratorFunction);
  if (value->IsGeneratorObject())   kinds |= (1ULL << Kind::kGeneratorObject  );
  if (value->IsPromise())           kinds |= (1ULL << Kind::kPromise          );
  if (value->IsMap())               kinds |= (1ULL << Kind::kMap              );
  if (value->IsSet())               kinds |= (1ULL << Kind::kSet              );
  if (value->IsMapIterator())       kinds |= (1ULL << Kind::kMapIterator      );
  if (value->IsSetIterator())       kinds |= (1ULL << Kind::kSetIterator      );
  if (value->IsWeakMap())           kinds |= (1ULL << Kind::kWeakMap          );
  if (value->IsWeakSet())           kinds |= (1ULL << Kind::kWeakSet          );
  if (value->IsArrayBuffer())       kinds |= (1ULL << Kind::kArrayBuffer      );
  if (value->IsArrayBufferView())   kinds |= (1ULL << Kind::kArrayBufferView  );
  if (value->IsTypedArray())        kinds |= (1ULL << Kind::kTypedArray       );
  if (value->IsUint8Array())        kinds |= (1ULL << Kind::kUint8Array       );
  if (value->IsUint8ClampedArray()) kinds |= (1ULL << Kind::kUint8ClampedArray);
  if (value->IsInt8Array())         kinds |= (1ULL << Kind::kInt8Array        );
  if (value->IsUint16Array())       kinds |= (1ULL << Kind::kUint16Array      );
  if (value->IsInt16Array())        kinds |= (1ULL << Kind::kInt16Array       );
  if (value->IsUint32Array())       kinds |= (1ULL << Kind::kUint32Array      );
  if (value->IsInt32Array())        kinds |= (1ULL << Kind::kInt32Array       );
  if (value->IsFloat32Array())      kinds |= (1ULL << Kind::kFloat32Array     );
  if (value->IsFloat64Array())      kinds |= (1ULL << Kind::kFloat64Array     );
  if (value->IsDataView())          kinds |= (1ULL << Kind::kDataView         );
  if (value->IsSharedArrayBuffer()) kinds |= (1ULL << Kind::kSharedArrayBuffer);
  if (value->IsProxy())             kinds |= (1ULL << Kind::kProxy            );
  // TODO(pmuir): do we need this?
  //if (value->IsWebAssemblyCompiledModule())
  //  kinds |= (1ULL << Kind::kWebAssemblyCompiledModule);

  return kinds;
}

inline ValueTuple v8_Value_ValueTuple() {
  return ValueTuple{ NULL, 0, NULL };
}

inline ValueTuple v8_Value_ValueTuple(v8::Isolate* isolate, v8::Local<v8::Value> value) {
  return ValueTuple{ new Value(isolate, value), v8_Value_KindsFromLocal(value) };
}

inline ValueTuple v8_Value_ValueTuple_Error(const v8::Local<v8::Value>& value) {
  return ValueTuple{ NULL, 0, v8_String_Create(value) };
}

#endif
