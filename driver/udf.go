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

func GetRenderFunc(filepath string, renderType string, renderData UdfRenderData) (func(w http.ResponseWriter, req *http.Request), error){
	rd := internaldriver.UdfRenderData(renderData)
	return internaldriver.GetRenderFunc(filepath , renderType, rd )
}
