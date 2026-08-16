package app

import(
 "bytes"
 "net/http"
 "testing"
)
func TestAutocompleteCompletesPluginAndSubcommandNames(t *testing.T){cfg:=configFor(t,"master.example.test:5050",nil);application,err:=New(cfg,http.DefaultClient);if err!=nil{t.Fatal(err)};var out bytes.Buffer;code:=application.Run([]string{"__autocomplete__","fr"},bytes.NewReader(nil),&out,&bytes.Buffer{});if code!=0||out.String()!="default\nframework\n"{t.Fatalf("top completion=%q",out.String())};out.Reset();code=application.Run([]string{"__autocomplete__","in","framework"},bytes.NewReader(nil),&out,&bytes.Buffer{});if code!=0||out.String()!="default\ninspect\n"{t.Fatalf("sub completion=%q",out.String())}}
