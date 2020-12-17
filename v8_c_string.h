
#ifndef V8_C_STRING_H
#define V8_C_STRING_H

#include <string>

inline std::string v8_String_ToStdString(v8::Isolate* isolate, v8::Local<v8::Value> value) {
  v8::String::Utf8Value s(isolate, value);

  if (s.length() == 0) {
    return "";
  }

  return *s;
}

inline v8::Local<v8::String> v8_String_FromString(v8::Isolate* isolate, const String& string) {
  v8::EscapableHandleScope handleScope(isolate);
  return handleScope.Escape(v8::String::NewFromUtf8(isolate, string.data, v8::NewStringType::kNormal, string.length).ToLocalChecked());
}

inline v8::Local<v8::String> v8_String_FromString(v8::Isolate* isolate, const std::string& string) {
  v8::EscapableHandleScope handleScope(isolate);
  return handleScope.Escape(v8::String::NewFromUtf8(isolate, string.c_str(), v8::NewStringType::kNormal, string.length()).ToLocalChecked());
}

inline v8::Local<v8::String> v8_String_FromString(v8::Isolate* isolate, const char* string) {
  v8::EscapableHandleScope handleScope(isolate);
  return handleScope.Escape(v8::String::NewFromUtf8(isolate, string, v8::NewStringType::kNormal, strlen(string)).ToLocalChecked());
}

inline String v8_String_Create(const v8::String::Utf8Value& src) {
  char* data = static_cast<char*>(malloc(src.length()));
  memcpy(data, *src, src.length());
  return (String){data, src.length()};
}

inline String v8_String_Create(v8::Isolate* isolate, const v8::Local<v8::Value>& val) {
  return v8_String_Create(v8::String::Utf8Value(isolate, val));
}

inline String v8_String_Create(const char* msg) {
  const char* data = strdup(msg);
  return (String){data, int(strlen(msg))};
}

inline String v8_String_Create(const std::string& src) {
  char* data = static_cast<char*>(malloc(src.length()));
  memcpy(data, src.data(), src.length());
  return (String){data, int(src.length())};
}

#endif
