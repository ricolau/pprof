package driver

import (
	internaldriver "github.com/google/pprof/internal/driver"
	"net/http"
)

type UdfRenderData internaldriver.UdfRenderData

//func GetRenderDataTemplate () internaldriver.UdfRenderData{
//	rd := internaldriver.UdfRenderData{}
//	return rd
//}

type RenderOption internaldriver.RenderOption

func GetRenderFunc(filepath string, renderType string, renderData UdfRenderData) (func(w http.ResponseWriter, req *http.Request), error) {
	rd := internaldriver.UdfRenderData(renderData)
	ro := internaldriver.RenderOption{}
	return internaldriver.GetRenderFunc(filepath, renderType, rd, ro)
}

func GetRenderFuncV2(filepath string, renderType string, renderData UdfRenderData, renderOption RenderOption) (func(w http.ResponseWriter, req *http.Request), error) {
	rd := internaldriver.UdfRenderData(renderData)
	ro := internaldriver.RenderOption(renderOption)
	return internaldriver.GetRenderFunc(filepath, renderType, rd, ro)
}
