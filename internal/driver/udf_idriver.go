package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/pprof/internal/binutils"
	"github.com/google/pprof/internal/graph"
	"github.com/google/pprof/internal/measurement"
	"github.com/google/pprof/internal/plugin"
	"github.com/google/pprof/internal/report"
	"github.com/google/pprof/internal/transport"
	"github.com/google/pprof/profile"
	"github.com/google/pprof/third_party/d3"
	"github.com/google/pprof/third_party/d3flamegraph"
	"html/template"
	"net/http"
	"os"
	"strings"
)

var singleTransport http.RoundTripper

type renderData map[string]string


func GetRenderFunc(filepath string, renderType string, renderData UdfRenderData) (func(w http.ResponseWriter, req *http.Request), error){

	if  singleTransport ==nil{
		o:=&plugin.Options{}
		o.Flagset = &GoFlags{}
		singleTransport = transport.New(o.Flagset)
	}

	cxt := context.Background()
	o:=&plugin.Options{}
	o.Flagset = &GoFlags{}
	o.HTTPTransport = singleTransport

	o = setDefaults(o)
	//src, cmd, err := initSource(cxt, filepath, o)
	src, _, _ := initSource(cxt, filepath, o)
	p, err := fetchProfiles(src, o)

	ui, err := makeWebInterface2(p, o)


	if err != nil {
		return nil, err
	}
	for n, c := range pprofCommands {
		ui.help[n] = c.description
	}
	for n, help := range configHelp {
		ui.help[n] = help
	}
	ui.help["details"] = "Show information about the profile and this view"
	ui.help["graph"] = "Display profile as a directed graph"
	ui.help["reset"] = "Show the entire profile"
	ui.help["save_config"] = "Save current settings"

	ui.renderData = renderData


	switch(renderType){
	case "top":
		return ui.top, nil
		break
	case "disasm":
		return ui.disasm, nil
		break
	case "source":
		return ui.source, nil
		break
	case "peek":
		return ui.peek, nil
		break
	case "flamegraph":
		return ui.flamegraph, nil
		break
	//case "saveconfig":
	//	return ui.dot, nil
	//	break
	//case "deleteconfig":
	//	return ui.dot, nil
	//	break
	//case "download":
	//	return ui.dot, nil
	//	break
	default:
		return ui.dot, nil
		break


	}
	return nil, errors.New("bad renderType:" + renderType)
	//
	//server := o.HTTPServer
	//if server == nil {
	//	server = defaultWebServer
	//}
	//args := &plugin.HTTPServerArgs{
	//	Hostport: net.JoinHostPort(host, strconv.Itoa(port)),
	//	Host:     host,
	//	Port:     port,
	//	Handlers: map[string]http.Handler{
	//		"/":             http.HandlerFunc(ui.dot),
	//		"/top":          http.HandlerFunc(ui.top),
	//		"/disasm":       http.HandlerFunc(ui.disasm),
	//		"/source":       http.HandlerFunc(ui.source),
	//		"/peek":         http.HandlerFunc(ui.peek),
	//		"/flamegraph":   http.HandlerFunc(ui.flamegraph),
	//		"/saveconfig":   http.HandlerFunc(ui.saveConfig),
	//		"/deleteconfig": http.HandlerFunc(ui.deleteConfig),
	//		"/download": http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	//			w.Header().Set("Content-Type", "application/vnd.google.protobuf+gzip")
	//			w.Header().Set("Content-Disposition", "attachment;filename=profile.pb.gz")
	//			p.Write(w)
	//		}),
	//	},
	//}
	//
	//url := "http://" + args.Hostport
	//
	//o.UI.Print("Serving web UI on ", url)
	//
	//if o.UI.WantBrowser() && !disableBrowser {
	//	go openBrowser(url, o)
	//}
	//return server(args)




}





func initRenderArgs(rd UdfRenderData) webArgs2{
	wd := webArgs2{}
	wd.Topurl = rd.Topurl
	wd.Graphurl = rd.Graphurl
	wd.Flamegraphurl = rd.Flamegraphurl
	wd.Peekurl = rd.Peekurl
	wd.Sourceurl = rd.Sourceurl
	wd.Disasmurl = rd.Disasmurl
	wd.Downloadurl = rd.Downloadurl

	return wd

}

// dot generates a web page containing an svg diagram.
func (ui *webInterface2) dot(w http.ResponseWriter, req *http.Request) {
	rpt, errList := ui.makeReport(w, req, []string{"svg"}, nil)
	if rpt == nil {
		return // error already reported
	}

	// Generate dot graph.
	g, config := report.GetDOT(rpt)
	legend := config.Labels
	config.Labels = nil
	dot := &bytes.Buffer{}
	graph.ComposeDot(dot, g, &graph.DotAttributes{}, config)

	// Convert to svg.
	svg, err := dotToSvg(dot.Bytes())
	if err != nil {
		http.Error(w, "Could not execute dot; may need to install graphviz.",
			http.StatusNotImplemented)
		ui.options.UI.PrintErr("Failed to execute dot. Is Graphviz installed?\n", err)
		return
	}

	// Get all node names into an array.
	nodes := []string{""} // dot starts with node numbered 1
	for _, n := range g.Nodes {
		nodes = append(nodes, n.Info.Name)
	}
	wd := initRenderArgs(ui.renderData)
	wd.HTMLBody = template.HTML(string(svg))
	wd.Nodes = nodes

	fmt.Println(wd.Peekurl,1111)
	ui.render(w, req, "graph", rpt, errList, legend, wd)
}

func (ui *webInterface2) top(w http.ResponseWriter, req *http.Request) {
	rpt, errList := ui.makeReport(w, req, []string{"top"}, func(cfg *config) {
		cfg.NodeCount = 500
	})
	if rpt == nil {
		return // error already reported
	}
	top, legend := report.TextItems(rpt)
	var nodes []string
	for _, item := range top {
		nodes = append(nodes, item.Name)
	}

	wd := initRenderArgs(ui.renderData)
	wd.Top = top
	wd.Nodes = nodes
	ui.render(w, req, "top", rpt, errList, legend, wd )
}

// disasm generates a web page containing disassembly.
func (ui *webInterface2) disasm(w http.ResponseWriter, req *http.Request) {
	args := []string{"disasm", req.URL.Query().Get("f")}
	rpt, errList := ui.makeReport(w, req, args, nil)
	if rpt == nil {
		return // error already reported
	}

	out := &bytes.Buffer{}
	if err := report.PrintAssembly(out, rpt, ui.options.Obj, maxEntries); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return
	}

	legend := report.ProfileLabels(rpt)
	wd := initRenderArgs(ui.renderData)
	wd.TextBody = out.String()
	ui.render(w, req, "plaintext", rpt, errList, legend, wd )

}

// source generates a web page containing source code annotated with profile
// data.
func (ui *webInterface2) source(w http.ResponseWriter, req *http.Request) {
	args := []string{"weblist", req.URL.Query().Get("f")}
	rpt, errList := ui.makeReport(w, req, args, nil)
	if rpt == nil {
		return // error already reported
	}

	// Generate source listing.
	var body bytes.Buffer
	if err := report.PrintWebList(&body, rpt, ui.options.Obj, maxEntries); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return
	}

	legend := report.ProfileLabels(rpt)

	wd := initRenderArgs(ui.renderData)
	wd.HTMLBody = template.HTML(body.String())
	ui.render(w, req, "sourcelisting", rpt, errList, legend, wd )
}

// peek generates a web page listing callers/callers.
func (ui *webInterface2) peek(w http.ResponseWriter, req *http.Request) {
	args := []string{"peek", req.URL.Query().Get("f")}
	rpt, errList := ui.makeReport(w, req, args, func(cfg *config) {
		cfg.Granularity = "lines"
	})
	if rpt == nil {
		return // error already reported
	}

	out := &bytes.Buffer{}
	if err := report.Generate(out, rpt, ui.options.Obj); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return
	}

	legend := report.ProfileLabels(rpt)

	wd := initRenderArgs(ui.renderData)
	wd.TextBody = out.String()
	ui.render(w, req, "plaintext", rpt, errList, legend, wd )

}




// flamegraph generates a web page containing a flamegraph.
func (ui *webInterface2) flamegraph(w http.ResponseWriter, req *http.Request) {
	// Force the call tree so that the graph is a tree.
	// Also do not trim the tree so that the flame graph contains all functions.
	rpt, errList := ui.makeReport(w, req, []string{"svg"}, func(cfg *config) {
		cfg.CallTree = true
		cfg.Trim = false
	})
	if rpt == nil {
		return // error already reported
	}

	// Generate dot graph.
	g, config := report.GetDOT(rpt)
	var nodes []*treeNode
	nroots := 0
	rootValue := int64(0)
	nodeArr := []string{}
	nodeMap := map[*graph.Node]*treeNode{}
	// Make all nodes and the map, collect the roots.
	for _, n := range g.Nodes {
		v := n.CumValue()
		fullName := n.Info.PrintableName()
		node := &treeNode{
			Name:      graph.ShortenFunctionName(fullName),
			FullName:  fullName,
			Cum:       v,
			CumFormat: config.FormatValue(v),
			Percent:   strings.TrimSpace(measurement.Percentage(v, config.Total)),
		}
		nodes = append(nodes, node)
		if len(n.In) == 0 {
			nodes[nroots], nodes[len(nodes)-1] = nodes[len(nodes)-1], nodes[nroots]
			nroots++
			rootValue += v
		}
		nodeMap[n] = node
		// Get all node names into an array.
		nodeArr = append(nodeArr, n.Info.Name)
	}
	// Populate the child links.
	for _, n := range g.Nodes {
		node := nodeMap[n]
		for child := range n.Out {
			node.Children = append(node.Children, nodeMap[child])
		}
	}

	rootNode := &treeNode{
		Name:      "root",
		FullName:  "root",
		Cum:       rootValue,
		CumFormat: config.FormatValue(rootValue),
		Percent:   strings.TrimSpace(measurement.Percentage(rootValue, config.Total)),
		Children:  nodes[0:nroots],
	}

	// JSON marshalling flame graph
	b, err := json.Marshal(rootNode)
	if err != nil {
		http.Error(w, "error serializing flame graph", http.StatusInternalServerError)
		ui.options.UI.PrintErr(err)
		return
	}
	wd := initRenderArgs(ui.renderData)
	wd.FlameGraph = template.JS(b)
	wd.Nodes = nodeArr
	ui.render(w, req, "flamegraph", rpt, errList, config.Labels, wd )

}




type UdfRenderData struct{
	Topurl string
	Graphurl string
	Flamegraphurl string
	Peekurl string
	Sourceurl string
	Disasmurl string
	Sampleurl string
	Cpuurl string
	Downloadurl string
}

// webArgs contains arguments passed to templates in webhtml.go.
type webArgs2 struct {
	Title       string
	Errors      []string
	Total       int64
	SampleTypes []string
	Legend      []string
	Help        map[string]string
	Nodes       []string
	HTMLBody    template.HTML
	TextBody    string
	Top         []report.TextItem
	FlameGraph  template.JS
	Configs     []configMenuEntry
	UdfRenderData
}
// render generates html using the named template based on the contents of data.
func (ui *webInterface2) render(w http.ResponseWriter, req *http.Request, tmpl string,
	rpt *report.Report, errList, legend []string, data webArgs2) {
	file := getFromLegend(legend, "File: ", "unknown")
	profile := getFromLegend(legend, "Type: ", "unknown")
	data.Title = file + " " + profile
	data.Errors = errList
	data.Total = rpt.Total()
	data.SampleTypes = sampleTypes(ui.prof)
	data.Legend = legend
	data.Help = ui.help
	data.Configs = configMenu(ui.settingsFile, *req.URL)

	html := &bytes.Buffer{}
	if err := ui.templates.ExecuteTemplate(html, tmpl, data); err != nil {
		http.Error(w, "internal template error", http.StatusInternalServerError)
		ui.options.UI.PrintErr(err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(html.Bytes())
}

// webInterface holds the state needed for serving a browser based interface.
type webInterface2 struct {
	prof         *profile.Profile
	options      *plugin.Options
	help         map[string]string
	templates    *template.Template
	settingsFile string
	renderData UdfRenderData
}


func makeWebInterface2(p *profile.Profile, opt *plugin.Options) (*webInterface2, error) {
	settingsFile, err := settingsFileName()
	if err != nil {
		return nil, err
	}
	templates := template.New("templategroup")
	addTemplates2(templates)
	report.AddSourceTemplates(templates)
	return &webInterface2{
		prof:         p,
		options:      opt,
		help:         make(map[string]string),
		templates:    templates,
		settingsFile: settingsFile,
	}, nil
}



func initSource(c context.Context, filepath string, o *plugin.Options) (*source, []string, error) {
	//flag := o.Flagset
	// Comparisons.

	ds1 := []*string{}
	flagDiffBase := &ds1

	ds2 := []*string{}
	flagBase := &ds2
	// Source options.

	ds3 := ""
	flagSymbolize := &ds3

	es := ""
	flagBuildID := &es

	ei := -1
	flagTimeout := &ei
	es2 := ""
	flagAddComment := &es2
	// CPU profile options
	ei2 :=-1
	flagSeconds := &ei2
	// Heap profile options

	df4 := false
	flagInUseSpace := &df4

	df5 :=false
	flagInUseObjects := &df5

	df6:=false
	flagAllocSpace := &df6

	df7 := false
	flagAllocObjects := &df7
	// Contention profile options

	defaultFalse := false
	flagTotalDelay := &defaultFalse

	df3 :=false
	flagContentions := &df3

	defaultFalse2 := false
	flagMeanDelay := &defaultFalse2

	es3 := os.Getenv("PPROF_TOOLS")
	flagTools := &es3

	es4 := ""
	flagHTTP := &es4

	df8 := false
	flagNoBrowser := &df8

	// Flags that set configuration properties.
	cfg := currentConfig()
	//configFlagSetter := installConfigFlags(flag, &cfg)

	flagCommands := make(map[string]*bool)
	flagParamCommands := make(map[string]*string)
	for name, cmd := range pprofCommands {
		if cmd.hasParam {
			ds := ""
			flagParamCommands[name] = &ds
		} else {
			df := false
			flagCommands[name] = &df
		}
	}



	var execName string
	// Recognize first argument as an executable or buildid override.

	arg0 := filepath
	args := []string{arg0}
	//if file, err := o.Obj.Open(arg0, 0, ^uint64(0), 0); err == nil {
	//	file.Close()
	//	execName = arg0
	//} else if *flagBuildID == "" && isBuildID(arg0) {
	//	*flagBuildID = arg0
	//}

	// Apply any specified flags to cfg.
	//if err := configFlagSetter(); err != nil {
	//	return nil, nil, err
	//}

	cmd, err := outputFormat(flagCommands, flagParamCommands)
	if err != nil {
		return nil, nil, err
	}
	if cmd != nil && *flagHTTP != "" {
		return nil, nil, errors.New("-http is not compatible with an output format on the command line")
	}

	if *flagNoBrowser && *flagHTTP == "" {
		return nil, nil, errors.New("-no_browser only makes sense with -http")
	}

	si := cfg.SampleIndex
	si = sampleIndex(flagTotalDelay, si, "delay", "-total_delay", o.UI)
	si = sampleIndex(flagMeanDelay, si, "delay", "-mean_delay", o.UI)
	si = sampleIndex(flagContentions, si, "contentions", "-contentions", o.UI)
	si = sampleIndex(flagInUseSpace, si, "inuse_space", "-inuse_space", o.UI)
	si = sampleIndex(flagInUseObjects, si, "inuse_objects", "-inuse_objects", o.UI)
	si = sampleIndex(flagAllocSpace, si, "alloc_space", "-alloc_space", o.UI)
	si = sampleIndex(flagAllocObjects, si, "alloc_objects", "-alloc_objects", o.UI)
	cfg.SampleIndex = si

	if *flagMeanDelay {
		cfg.Mean = true
	}

	source := &source{
		Sources:            args,
		ExecName:           execName,
		BuildID:            *flagBuildID,
		Seconds:            *flagSeconds,
		Timeout:            *flagTimeout,
		Symbolize:          *flagSymbolize,
		HTTPHostport:       *flagHTTP,
		HTTPDisableBrowser: *flagNoBrowser,
		Comment:            *flagAddComment,
	}

	if err := source.addBaseProfiles(*flagBase, *flagDiffBase); err != nil {
		return nil, nil, err
	}

	normalize := cfg.Normalize
	if normalize && len(source.Base) == 0 {
		return nil, nil, errors.New("must have base profile to normalize by")
	}
	source.Normalize = normalize

	if bu, ok := o.Obj.(*binutils.Binutils); ok {
		bu.SetTools(*flagTools)
	}

	setCurrentConfig(cfg)
	return source, cmd, nil
}

// makeReport generates a report for the specified command.
// If configEditor is not null, it is used to edit the config used for the report.
func (ui *webInterface2) makeReport(w http.ResponseWriter, req *http.Request,
	cmd []string, configEditor func(*config)) (*report.Report, []string) {
	cfg := currentConfig()
	if err := cfg.applyURL(req.URL.Query()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return nil, nil
	}
	if configEditor != nil {
		configEditor(&cfg)
	}
	catcher := &errorCatcher{UI: ui.options.UI}
	options := *ui.options
	options.UI = catcher
	_, rpt, err := generateRawReport(ui.prof, cmd, cfg, &options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return nil, nil
	}
	return rpt, catcher.errors
}

// addTemplates adds a set of template definitions to templates.
func addTemplates2(templates *template.Template) {
	template.Must(templates.Parse(`{{define "d3script"}}` + d3.JSSource + `{{end}}`))
	template.Must(templates.Parse(`{{define "d3flamegraphscript"}}` + d3flamegraph.JSSource + `{{end}}`))
	template.Must(templates.Parse(`{{define "d3flamegraphcss"}}` + d3flamegraph.CSSSource + `{{end}}`))
	template.Must(templates.Parse(`
{{define "css"}}
<style type="text/css">
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}
html, body {
  height: 100%;
}
body {
  font-family: 'Roboto', -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol';
  font-size: 13px;
  line-height: 1.4;
  display: flex;
  flex-direction: column;
}
a {
  color: #2a66d9;
}
.header {
  display: flex;
  align-items: center;
  height: 44px;
  min-height: 44px;
  background-color: #eee;
  color: #212121;
  padding: 0 1rem;
}
.header > div {
  margin: 0 0.125em;
}
.header .title h1 {
  font-size: 1.75em;
  margin-right: 1rem;
  margin-bottom: 4px;
}
.header .title a {
  color: #212121;
  text-decoration: none;
}
.header .title a:hover {
  text-decoration: underline;
}
.header .description {
  width: 100%;
  text-align: right;
  white-space: nowrap;
}
@media screen and (max-width: 799px) {
  .header input {
    display: none;
  }
}
#detailsbox {
  display: none;
  z-index: 1;
  position: fixed;
  top: 40px;
  right: 20px;
  background-color: #ffffff;
  box-shadow: 0 1px 5px rgba(0,0,0,.3);
  line-height: 24px;
  padding: 1em;
  text-align: left;
}
.header input {
  background: white url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' style='pointer-events:none;display:block;width:100%25;height:100%25;fill:%23757575'%3E%3Cpath d='M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61.0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z'/%3E%3C/svg%3E") no-repeat 4px center/20px 20px;
  border: 1px solid #d1d2d3;
  border-radius: 2px 0 0 2px;
  padding: 0.25em;
  padding-left: 28px;
  margin-left: 1em;
  font-family: 'Roboto', 'Noto', sans-serif;
  font-size: 1em;
  line-height: 24px;
  color: #212121;
}
.downArrow {
  border-top: .36em solid #ccc;
  border-left: .36em solid transparent;
  border-right: .36em solid transparent;
  margin-bottom: .05em;
  margin-left: .5em;
  transition: border-top-color 200ms;
}
.menu-item {
  height: 100%;
  text-transform: uppercase;
  font-family: 'Roboto Medium', -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol';
  position: relative;
}
.menu-item .menu-name:hover {
  opacity: 0.75;
}
.menu-item .menu-name:hover .downArrow {
  border-top-color: #666;
}
.menu-name {
  height: 100%;
  padding: 0 0.5em;
  display: flex;
  align-items: center;
  justify-content: center;
}
.menu-name a {
  text-decoration: none;
  color: #212121;
}
.submenu {
  display: none;
  z-index: 1;
  margin-top: -4px;
  min-width: 10em;
  position: absolute;
  left: 0px;
  background-color: white;
  box-shadow: 0 1px 5px rgba(0,0,0,.3);
  font-size: 100%;
  text-transform: none;
}
.menu-item, .submenu {
  user-select: none;
  -moz-user-select: none;
  -ms-user-select: none;
  -webkit-user-select: none;
}
.submenu hr {
  border: 0;
  border-top: 2px solid #eee;
}
.submenu a {
  display: block;
  padding: .5em 1em;
  text-decoration: none;
}
.submenu a:hover, .submenu a.active {
  color: white;
  background-color: #6b82d6;
}
.submenu a.disabled {
  color: gray;
  pointer-events: none;
}
.menu-check-mark {
  position: absolute;
  left: 2px;
}
.menu-delete-btn {
  position: absolute;
  right: 2px;
}

{{/* Used to disable events when a modal dialog is displayed */}}
#dialog-overlay {
  display: none;
  position: fixed;
  left: 0px;
  top: 0px;
  width: 100%;
  height: 100%;
  background-color: rgba(1,1,1,0.1);
}

.dialog {
  {{/* Displayed centered horizontally near the top */}}
  display: none;
  position: fixed;
  margin: 0px;
  top: 60px;
  left: 50%;
  transform: translateX(-50%);

  z-index: 3;
  font-size: 125%;
  background-color: #ffffff;
  box-shadow: 0 1px 5px rgba(0,0,0,.3);
}
.dialog-header {
  font-size: 120%;
  border-bottom: 1px solid #CCCCCC;
  width: 100%;
  text-align: center;
  background: #EEEEEE;
  user-select: none;
}
.dialog-footer {
  border-top: 1px solid #CCCCCC;
  width: 100%;
  text-align: right;
  padding: 10px;
}
.dialog-error {
  margin: 10px;
  color: red;
}
.dialog input {
  margin: 10px;
  font-size: inherit;
}
.dialog button {
  margin-left: 10px;
  font-size: inherit;
}
#save-dialog, #delete-dialog {
  width: 50%;
  max-width: 20em;
}
#delete-prompt {
  padding: 10px;
}

#content {
  overflow-y: scroll;
  padding: 1em;
}
#top {
  overflow-y: scroll;
}
#graph {
  overflow: hidden;
}
#graph svg {
  width: 100%;
  height: auto;
  padding: 10px;
}
#content.source .filename {
  margin-top: 0;
  margin-bottom: 1em;
  font-size: 120%;
}
#content.source pre {
  margin-bottom: 3em;
}
table {
  border-spacing: 0px;
  width: 100%;
  padding-bottom: 1em;
  white-space: nowrap;
}
table thead {
  font-family: 'Roboto Medium', -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol';
}
table tr th {
  position: sticky;
  top: 0;
  background-color: #ddd;
  text-align: right;
  padding: .3em .5em;
}
table tr td {
  padding: .3em .5em;
  text-align: right;
}
#top table tr th:nth-child(6),
#top table tr th:nth-child(7),
#top table tr td:nth-child(6),
#top table tr td:nth-child(7) {
  text-align: left;
}
#top table tr td:nth-child(6) {
  width: 100%;
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}
#flathdr1, #flathdr2, #cumhdr1, #cumhdr2, #namehdr {
  cursor: ns-resize;
}
.hilite {
  background-color: #ebf5fb;
  font-weight: bold;
}
</style>
{{end}}

{{define "header"}}
<div class="header">
  <div class="title">
    <h1><a href="/">pprof</a></h1>
  </div>

  <div id="view" class="menu-item">
    <div class="menu-name">
      View
      <i class="downArrow"></i>
    </div>
    <div class="submenu">
      <a title="{{.Help.top}}"  href="{{.Topurl}}" id="topbtn">Top</a>
      <a title="{{.Help.graph}}" href="{{.Graphurl}}" id="graphbtn">Graph</a>
      <a title="{{.Help.flamegraph}}" href="{{.Flamegraphurl}}" id="flamegraph">Flame Graph</a>
      <a title="{{.Help.peek}}" href="{{.Peekurl}}" id="peek">Peek</a>
      <a title="{{.Help.list}}" href="{{.Sourceurl}}" id="list">Source</a>
      <a title="{{.Help.disasm}}" href="{{.Disasmurl}}" id="disasm">Disassemble</a>
    </div>
  </div>

  {{$sampleLen := len .SampleTypes}}
  {{if gt $sampleLen 1}}
  <div id="sample" class="menu-item">
    <div class="menu-name">
      Sample
      <i class="downArrow"></i>
    </div>
    <div class="submenu">
      {{range .SampleTypes}}
      <a href="?si={{.}}" id="{{.}}">{{.}}</a>
      {{end}}
    </div>
  </div>
  {{end}}

  <div id="refine" class="menu-item">
    <div class="menu-name">
      Refine
      <i class="downArrow"></i>
    </div>
    <div class="submenu">
      <a title="{{.Help.focus}}" href="?" id="focus">Focus</a>
      <a title="{{.Help.ignore}}" href="?" id="ignore">Ignore</a>
      <a title="{{.Help.hide}}" href="?" id="hide">Hide</a>
      <a title="{{.Help.show}}" href="?" id="show">Show</a>
      <a title="{{.Help.show_from}}" href="?" id="show-from">Show from</a>
      <hr>
      <a title="{{.Help.reset}}" href="?">Reset</a>
    </div>
  </div>

  <div id="config" class="menu-item">
    <div class="menu-name">
      Config
      <i class="downArrow"></i>
    </div>
    <div class="submenu">
      <a title="{{.Help.save_config}}" id="save-config">Save as ...</a>
      <hr>
      {{range .Configs}}
        <a href="{{.URL}}">
          {{if .Current}}<span class="menu-check-mark">✓</span>{{end}}
          {{.Name}}
          {{if .UserConfig}}<span class="menu-delete-btn" data-config={{.Name}}>🗙</span>{{end}}
        </a>
      {{end}}
    </div>
  </div>

  <div id="download" class="menu-item">
    <div class="menu-name">
      <a href="{{.Downloadurl}}">Download</a>
    </div>
  </div>

  <div>
    <input id="search" type="text" placeholder="Search regexp" autocomplete="off" autocapitalize="none" size=40>
  </div>

  <div class="description">
    <a title="{{.Help.details}}" href="#" id="details">{{.Title}}</a>
    <div id="detailsbox">
      {{range .Legend}}<div>{{.}}</div>{{end}}
    </div>
  </div>
</div>

<div id="dialog-overlay"></div>

<div class="dialog" id="save-dialog">
  <div class="dialog-header">Save options as</div>
  <datalist id="config-list">
    {{range .Configs}}{{if .UserConfig}}<option value="{{.Name}}" />{{end}}{{end}}
  </datalist>
  <input id="save-name" type="text" list="config-list" placeholder="New config" />
  <div class="dialog-footer">
    <span class="dialog-error" id="save-error"></span>
    <button id="save-cancel">Cancel</button>
    <button id="save-confirm">Save</button>
  </div>
</div>

<div class="dialog" id="delete-dialog">
  <div class="dialog-header" id="delete-dialog-title">Delete config</div>
  <div id="delete-prompt"></div>
  <div class="dialog-footer">
    <span class="dialog-error" id="delete-error"></span>
    <button id="delete-cancel">Cancel</button>
    <button id="delete-confirm">Delete</button>
  </div>
</div>

<div id="errors">{{range .Errors}}<div>{{.}}</div>{{end}}</div>
{{end}}

{{define "graph" -}}
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  {{template "css" .}}
</head>
<body>
  {{template "header" .}}
  <div id="graph">
    {{.HTMLBody}}
  </div>
  {{template "script" .}}
  <script>viewer(new URL(window.location.href), {{.Nodes}});</script>
</body>
</html>
{{end}}

{{define "script"}}
<script>
// Make svg pannable and zoomable.
// Call clickHandler(t) if a click event is caught by the pan event handlers.
function initPanAndZoom(svg, clickHandler) {
  'use strict';

  // Current mouse/touch handling mode
  const IDLE = 0;
  const MOUSEPAN = 1;
  const TOUCHPAN = 2;
  const TOUCHZOOM = 3;
  let mode = IDLE;

  // State needed to implement zooming.
  let currentScale = 1.0;
  const initWidth = svg.viewBox.baseVal.width;
  const initHeight = svg.viewBox.baseVal.height;

  // State needed to implement panning.
  let panLastX = 0;      // Last event X coordinate
  let panLastY = 0;      // Last event Y coordinate
  let moved = false;     // Have we seen significant movement
  let touchid = null;    // Current touch identifier

  // State needed for pinch zooming
  let touchid2 = null;     // Second id for pinch zooming
  let initGap = 1.0;       // Starting gap between two touches
  let initScale = 1.0;     // currentScale when pinch zoom started
  let centerPoint = null;  // Center point for scaling

  // Convert event coordinates to svg coordinates.
  function toSvg(x, y) {
    const p = svg.createSVGPoint();
    p.x = x;
    p.y = y;
    let m = svg.getCTM();
    if (m == null) m = svg.getScreenCTM(); // Firefox workaround.
    return p.matrixTransform(m.inverse());
  }

  // Change the scaling for the svg to s, keeping the point denoted
  // by u (in svg coordinates]) fixed at the same screen location.
  function rescale(s, u) {
    // Limit to a good range.
    if (s < 0.2) s = 0.2;
    if (s > 10.0) s = 10.0;

    currentScale = s;

    // svg.viewBox defines the visible portion of the user coordinate
    // system.  So to magnify by s, divide the visible portion by s,
    // which will then be stretched to fit the viewport.
    const vb = svg.viewBox;
    const w1 = vb.baseVal.width;
    const w2 = initWidth / s;
    const h1 = vb.baseVal.height;
    const h2 = initHeight / s;
    vb.baseVal.width = w2;
    vb.baseVal.height = h2;

    // We also want to adjust vb.baseVal.x so that u.x remains at same
    // screen X coordinate.  In other words, want to change it from x1 to x2
    // so that:
    //     (u.x - x1) / w1 = (u.x - x2) / w2
    // Simplifying that, we get
    //     (u.x - x1) * (w2 / w1) = u.x - x2
    //     x2 = u.x - (u.x - x1) * (w2 / w1)
    vb.baseVal.x = u.x - (u.x - vb.baseVal.x) * (w2 / w1);
    vb.baseVal.y = u.y - (u.y - vb.baseVal.y) * (h2 / h1);
  }

  function handleWheel(e) {
    if (e.deltaY == 0) return;
    // Change scale factor by 1.1 or 1/1.1
    rescale(currentScale * (e.deltaY < 0 ? 1.1 : (1/1.1)),
            toSvg(e.offsetX, e.offsetY));
  }

  function setMode(m) {
    mode = m;
    touchid = null;
    touchid2 = null;
  }

  function panStart(x, y) {
    moved = false;
    panLastX = x;
    panLastY = y;
  }

  function panMove(x, y) {
    let dx = x - panLastX;
    let dy = y - panLastY;
    if (Math.abs(dx) <= 2 && Math.abs(dy) <= 2) return; // Ignore tiny moves

    moved = true;
    panLastX = x;
    panLastY = y;

    // Firefox workaround: get dimensions from parentNode.
    const swidth = svg.clientWidth || svg.parentNode.clientWidth;
    const sheight = svg.clientHeight || svg.parentNode.clientHeight;

    // Convert deltas from screen space to svg space.
    dx *= (svg.viewBox.baseVal.width / swidth);
    dy *= (svg.viewBox.baseVal.height / sheight);

    svg.viewBox.baseVal.x -= dx;
    svg.viewBox.baseVal.y -= dy;
  }

  function handleScanStart(e) {
    if (e.button != 0) return; // Do not catch right-clicks etc.
    setMode(MOUSEPAN);
    panStart(e.clientX, e.clientY);
    e.preventDefault();
    svg.addEventListener('mousemove', handleScanMove);
  }

  function handleScanMove(e) {
    if (e.buttons == 0) {
      // Missed an end event, perhaps because mouse moved outside window.
      setMode(IDLE);
      svg.removeEventListener('mousemove', handleScanMove);
      return;
    }
    if (mode == MOUSEPAN) panMove(e.clientX, e.clientY);
  }

  function handleScanEnd(e) {
    if (mode == MOUSEPAN) panMove(e.clientX, e.clientY);
    setMode(IDLE);
    svg.removeEventListener('mousemove', handleScanMove);
    if (!moved) clickHandler(e.target);
  }

  // Find touch object with specified identifier.
  function findTouch(tlist, id) {
    for (const t of tlist) {
      if (t.identifier == id) return t;
    }
    return null;
  }

  // Return distance between two touch points
  function touchGap(t1, t2) {
    const dx = t1.clientX - t2.clientX;
    const dy = t1.clientY - t2.clientY;
    return Math.hypot(dx, dy);
  }

  function handleTouchStart(e) {
    if (mode == IDLE && e.changedTouches.length == 1) {
      // Start touch based panning
      const t = e.changedTouches[0];
      setMode(TOUCHPAN);
      touchid = t.identifier;
      panStart(t.clientX, t.clientY);
      e.preventDefault();
    } else if (mode == TOUCHPAN && e.touches.length == 2) {
      // Start pinch zooming
      setMode(TOUCHZOOM);
      const t1 = e.touches[0];
      const t2 = e.touches[1];
      touchid = t1.identifier;
      touchid2 = t2.identifier;
      initScale = currentScale;
      initGap = touchGap(t1, t2);
      centerPoint = toSvg((t1.clientX + t2.clientX) / 2,
                          (t1.clientY + t2.clientY) / 2);
      e.preventDefault();
    }
  }

  function handleTouchMove(e) {
    if (mode == TOUCHPAN) {
      const t = findTouch(e.changedTouches, touchid);
      if (t == null) return;
      if (e.touches.length != 1) {
        setMode(IDLE);
        return;
      }
      panMove(t.clientX, t.clientY);
      e.preventDefault();
    } else if (mode == TOUCHZOOM) {
      // Get two touches; new gap; rescale to ratio.
      const t1 = findTouch(e.touches, touchid);
      const t2 = findTouch(e.touches, touchid2);
      if (t1 == null || t2 == null) return;
      const gap = touchGap(t1, t2);
      rescale(initScale * gap / initGap, centerPoint);
      e.preventDefault();
    }
  }

  function handleTouchEnd(e) {
    if (mode == TOUCHPAN) {
      const t = findTouch(e.changedTouches, touchid);
      if (t == null) return;
      panMove(t.clientX, t.clientY);
      setMode(IDLE);
      e.preventDefault();
      if (!moved) clickHandler(t.target);
    } else if (mode == TOUCHZOOM) {
      setMode(IDLE);
      e.preventDefault();
    }
  }

  svg.addEventListener('mousedown', handleScanStart);
  svg.addEventListener('mouseup', handleScanEnd);
  svg.addEventListener('touchstart', handleTouchStart);
  svg.addEventListener('touchmove', handleTouchMove);
  svg.addEventListener('touchend', handleTouchEnd);
  svg.addEventListener('wheel', handleWheel, true);
}

function initMenus() {
  'use strict';

  let activeMenu = null;
  let activeMenuHdr = null;

  function cancelActiveMenu() {
    if (activeMenu == null) return;
    activeMenu.style.display = 'none';
    activeMenu = null;
    activeMenuHdr = null;
  }

  // Set click handlers on every menu header.
  for (const menu of document.getElementsByClassName('submenu')) {
    const hdr = menu.parentElement;
    if (hdr == null) return;
    if (hdr.classList.contains('disabled')) return;
    function showMenu(e) {
      // menu is a child of hdr, so this event can fire for clicks
      // inside menu. Ignore such clicks.
      if (e.target.parentElement != hdr) return;
      activeMenu = menu;
      activeMenuHdr = hdr;
      menu.style.display = 'block';
    }
    hdr.addEventListener('mousedown', showMenu);
    hdr.addEventListener('touchstart', showMenu);
  }

  // If there is an active menu and a down event outside, retract the menu.
  for (const t of ['mousedown', 'touchstart']) {
    document.addEventListener(t, (e) => {
      // Note: to avoid unnecessary flicker, if the down event is inside
      // the active menu header, do not retract the menu.
      if (activeMenuHdr != e.target.closest('.menu-item')) {
        cancelActiveMenu();
      }
    }, { passive: true, capture: true });
  }

  // If there is an active menu and an up event inside, retract the menu.
  document.addEventListener('mouseup', (e) => {
    if (activeMenu == e.target.closest('.submenu')) {
      cancelActiveMenu();
    }
  }, { passive: true, capture: true });
}

function sendURL(method, url, done) {
  fetch(url.toString(), {method: method})
      .then((response) => { done(response.ok); })
      .catch((error) => { done(false); });
}

// Initialize handlers for saving/loading configurations.
function initConfigManager() {
  'use strict';

  // Initialize various elements.
  function elem(id) {
    const result = document.getElementById(id);
    if (!result) console.warn('element ' + id + ' not found');
    return result;
  }
  const overlay = elem('dialog-overlay');
  const saveDialog = elem('save-dialog');
  const saveInput = elem('save-name');
  const saveError = elem('save-error');
  const delDialog = elem('delete-dialog');
  const delPrompt = elem('delete-prompt');
  const delError = elem('delete-error');

  let currentDialog = null;
  let currentDeleteTarget = null;

  function showDialog(dialog) {
    if (currentDialog != null) {
      overlay.style.display = 'none';
      currentDialog.style.display = 'none';
    }
    currentDialog = dialog;
    if (dialog != null) {
      overlay.style.display = 'block';
      dialog.style.display = 'block';
    }
  }

  function cancelDialog(e) {
    showDialog(null);
  }

  // Show dialog for saving the current config.
  function showSaveDialog(e) {
    saveError.innerText = '';
    showDialog(saveDialog);
    saveInput.focus();
  }

  // Commit save config.
  function commitSave(e) {
    const name = saveInput.value;
    const url = new URL(document.URL);
    // Set path relative to existing path.
    url.pathname = new URL('./saveconfig', document.URL).pathname;
    url.searchParams.set('config', name);
    saveError.innerText = '';
    sendURL('POST', url, (ok) => {
      if (!ok) {
        saveError.innerText = 'Save failed';
      } else {
        showDialog(null);
        location.reload();  // Reload to show updated config menu
      }
    });
  }

  function handleSaveInputKey(e) {
    if (e.key === 'Enter') commitSave(e);
  }

  function deleteConfig(e, elem) {
    e.preventDefault();
    const config = elem.dataset.config;
    delPrompt.innerText = 'Delete ' + config + '?';
    currentDeleteTarget = elem;
    showDialog(delDialog);
  }

  function commitDelete(e, elem) {
    if (!currentDeleteTarget) return;
    const config = currentDeleteTarget.dataset.config;
    const url = new URL('./deleteconfig', document.URL);
    url.searchParams.set('config', config);
    delError.innerText = '';
    sendURL('DELETE', url, (ok) => {
      if (!ok) {
        delError.innerText = 'Delete failed';
        return;
      }
      showDialog(null);
      // Remove menu entry for this config.
      if (currentDeleteTarget && currentDeleteTarget.parentElement) {
        currentDeleteTarget.parentElement.remove();
      }
    });
  }

  // Bind event on elem to fn.
  function bind(event, elem, fn) {
    if (elem == null) return;
    elem.addEventListener(event, fn);
    if (event == 'click') {
      // Also enable via touch.
      elem.addEventListener('touchstart', fn);
    }
  }

  bind('click', elem('save-config'), showSaveDialog);
  bind('click', elem('save-cancel'), cancelDialog);
  bind('click', elem('save-confirm'), commitSave);
  bind('keydown', saveInput, handleSaveInputKey);

  bind('click', elem('delete-cancel'), cancelDialog);
  bind('click', elem('delete-confirm'), commitDelete);

  // Activate deletion button for all config entries in menu.
  for (const del of Array.from(document.getElementsByClassName('menu-delete-btn'))) {
    bind('click', del, (e) => {
      deleteConfig(e, del);
    });
  }
}

function viewer(baseUrl, nodes) {
  'use strict';

  // Elements
  const search = document.getElementById('search');
  const graph0 = document.getElementById('graph0');
  const svg = (graph0 == null ? null : graph0.parentElement);
  const toptable = document.getElementById('toptable');

  let regexpActive = false;
  let selected = new Map();
  let origFill = new Map();
  let searchAlarm = null;
  let buttonsEnabled = true;

  function handleDetails(e) {
    e.preventDefault();
    const detailsText = document.getElementById('detailsbox');
    if (detailsText != null) {
      if (detailsText.style.display === 'block') {
        detailsText.style.display = 'none';
      } else {
        detailsText.style.display = 'block';
      }
    }
  }

  function handleKey(e) {
    if (e.keyCode != 13) return;
    setHrefParams(window.location, function (params) {
      params.set('f', search.value);
    });
    e.preventDefault();
  }

  function handleSearch() {
    // Delay expensive processing so a flurry of key strokes is handled once.
    if (searchAlarm != null) {
      clearTimeout(searchAlarm);
    }
    searchAlarm = setTimeout(selectMatching, 300);

    regexpActive = true;
    updateButtons();
  }

  function selectMatching() {
    searchAlarm = null;
    let re = null;
    if (search.value != '') {
      try {
        re = new RegExp(search.value);
      } catch (e) {
        // TODO: Display error state in search box
        return;
      }
    }

    function match(text) {
      return re != null && re.test(text);
    }

    // drop currently selected items that do not match re.
    selected.forEach(function(v, n) {
      if (!match(nodes[n])) {
        unselect(n, document.getElementById('node' + n));
      }
    })

    // add matching items that are not currently selected.
    if (nodes) {
      for (let n = 0; n < nodes.length; n++) {
        if (!selected.has(n) && match(nodes[n])) {
          select(n, document.getElementById('node' + n));
        }
      }
    }

    updateButtons();
  }

  function toggleSvgSelect(elem) {
    // Walk up to immediate child of graph0
    while (elem != null && elem.parentElement != graph0) {
      elem = elem.parentElement;
    }
    if (!elem) return;

    // Disable regexp mode.
    regexpActive = false;

    const n = nodeId(elem);
    if (n < 0) return;
    if (selected.has(n)) {
      unselect(n, elem);
    } else {
      select(n, elem);
    }
    updateButtons();
  }

  function unselect(n, elem) {
    if (elem == null) return;
    selected.delete(n);
    setBackground(elem, false);
  }

  function select(n, elem) {
    if (elem == null) return;
    selected.set(n, true);
    setBackground(elem, true);
  }

  function nodeId(elem) {
    const id = elem.id;
    if (!id) return -1;
    if (!id.startsWith('node')) return -1;
    const n = parseInt(id.slice(4), 10);
    if (isNaN(n)) return -1;
    if (n < 0 || n >= nodes.length) return -1;
    return n;
  }

  function setBackground(elem, set) {
    // Handle table row highlighting.
    if (elem.nodeName == 'TR') {
      elem.classList.toggle('hilite', set);
      return;
    }

    // Handle svg element highlighting.
    const p = findPolygon(elem);
    if (p != null) {
      if (set) {
        origFill.set(p, p.style.fill);
        p.style.fill = '#ccccff';
      } else if (origFill.has(p)) {
        p.style.fill = origFill.get(p);
      }
    }
  }

  function findPolygon(elem) {
    if (elem.localName == 'polygon') return elem;
    for (const c of elem.children) {
      const p = findPolygon(c);
      if (p != null) return p;
    }
    return null;
  }

  // convert a string to a regexp that matches that string.
  function quotemeta(str) {
    return str.replace(/([\\\.?+*\[\](){}|^$])/g, '\\$1');
  }

  function setSampleIndexLink(id) {
    const elem = document.getElementById(id);
    if (elem != null) {
      setHrefParams(elem, function (params) {
        params.set("si", id);
      });
    }
  }

  // Update id's href to reflect current selection whenever it is
  // liable to be followed.
  function makeSearchLinkDynamic(id) {
    const elem = document.getElementById(id);
    if (elem == null) return;

    // Most links copy current selection into the 'f' parameter,
    // but Refine menu links are different.
    let param = 'f';
    if (id == 'ignore') param = 'i';
    if (id == 'hide') param = 'h';
    if (id == 'show') param = 's';
    if (id == 'show-from') param = 'sf';

    // We update on mouseenter so middle-click/right-click work properly.
    elem.addEventListener('mouseenter', updater);
    elem.addEventListener('touchstart', updater);

    function updater() {
      // The selection can be in one of two modes: regexp-based or
      // list-based.  Construct regular expression depending on mode.
      let re = regexpActive
        ? search.value
        : Array.from(selected.keys()).map(key => quotemeta(nodes[key])).join('|');

      setHrefParams(elem, function (params) {
        if (re != '') {
          // For focus/show/show-from, forget old parameter. For others, add to re.
          if (param != 'f' && param != 's' && param != 'sf' && params.has(param)) {
            const old = params.get(param);
            if (old != '') {
              re += '|' + old;
            }
          }
          params.set(param, re);
        } else {
          params.delete(param);
        }
      });
    }
  }

  function setHrefParams(elem, paramSetter) {
    let url = new URL(elem.href);
    url.hash = '';

    // Copy params from this page's URL.
    const params = url.searchParams;
    for (const p of new URLSearchParams(window.location.search)) {
      params.set(p[0], p[1]);
    }

    // Give the params to the setter to modify.
    paramSetter(params);

    elem.href = url.toString();
  }

  function handleTopClick(e) {
    // Walk back until we find TR and then get the Name column (index 5)
    let elem = e.target;
    while (elem != null && elem.nodeName != 'TR') {
      elem = elem.parentElement;
    }
    if (elem == null || elem.children.length < 6) return;

    e.preventDefault();
    const tr = elem;
    const td = elem.children[5];
    if (td.nodeName != 'TD') return;
    const name = td.innerText;
    const index = nodes.indexOf(name);
    if (index < 0) return;

    // Disable regexp mode.
    regexpActive = false;

    if (selected.has(index)) {
      unselect(index, elem);
    } else {
      select(index, elem);
    }
    updateButtons();
  }

  function updateButtons() {
    const enable = (search.value != '' || selected.size != 0);
    if (buttonsEnabled == enable) return;
    buttonsEnabled = enable;
    for (const id of ['focus', 'ignore', 'hide', 'show', 'show-from']) {
      const link = document.getElementById(id);
      if (link != null) {
        link.classList.toggle('disabled', !enable);
      }
    }
  }

  // Initialize button states
  updateButtons();

  // Setup event handlers
  initMenus();
  if (svg != null) {
    initPanAndZoom(svg, toggleSvgSelect);
  }
  if (toptable != null) {
    toptable.addEventListener('mousedown', handleTopClick);
    toptable.addEventListener('touchstart', handleTopClick);
  }

  const ids = ['topbtn', 'graphbtn', 'flamegraph', 'peek', 'list', 'disasm',
               'focus', 'ignore', 'hide', 'show', 'show-from'];
  ids.forEach(makeSearchLinkDynamic);

  const sampleIDs = [{{range .SampleTypes}}'{{.}}', {{end}}];
  sampleIDs.forEach(setSampleIndexLink);

  // Bind action to button with specified id.
  function addAction(id, action) {
    const btn = document.getElementById(id);
    if (btn != null) {
      btn.addEventListener('click', action);
      btn.addEventListener('touchstart', action);
    }
  }

  addAction('details', handleDetails);
  initConfigManager();

  search.addEventListener('input', handleSearch);
  search.addEventListener('keydown', handleKey);

  // Give initial focus to main container so it can be scrolled using keys.
  const main = document.getElementById('bodycontainer');
  if (main) {
    main.focus();
  }
}
</script>
{{end}}

{{define "top" -}}
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  {{template "css" .}}
  <style type="text/css">
  </style>
</head>
<body>
  {{template "header" .}}
  <div id="top">
    <table id="toptable">
      <thead>
        <tr>
          <th id="flathdr1">Flat</th>
          <th id="flathdr2">Flat%</th>
          <th>Sum%</th>
          <th id="cumhdr1">Cum</th>
          <th id="cumhdr2">Cum%</th>
          <th id="namehdr">Name</th>
          <th>Inlined?</th>
        </tr>
      </thead>
      <tbody id="rows"></tbody>
    </table>
  </div>
  {{template "script" .}}
  <script>
    function makeTopTable(total, entries) {
      const rows = document.getElementById('rows');
      if (rows == null) return;

      // Store initial index in each entry so we have stable node ids for selection.
      for (let i = 0; i < entries.length; i++) {
        entries[i].Id = 'node' + i;
      }

      // Which column are we currently sorted by and in what order?
      let currentColumn = '';
      let descending = false;
      sortBy('Flat');

      function sortBy(column) {
        // Update sort criteria
        if (column == currentColumn) {
          descending = !descending; // Reverse order
        } else {
          currentColumn = column;
          descending = (column != 'Name');
        }

        // Sort according to current criteria.
        function cmp(a, b) {
          const av = a[currentColumn];
          const bv = b[currentColumn];
          if (av < bv) return -1;
          if (av > bv) return +1;
          return 0;
        }
        entries.sort(cmp);
        if (descending) entries.reverse();

        function addCell(tr, val) {
          const td = document.createElement('td');
          td.textContent = val;
          tr.appendChild(td);
        }

        function percent(v) {
          return (v * 100.0 / total).toFixed(2) + '%';
        }

        // Generate rows
        const fragment = document.createDocumentFragment();
        let sum = 0;
        for (const row of entries) {
          const tr = document.createElement('tr');
          tr.id = row.Id;
          sum += row.Flat;
          addCell(tr, row.FlatFormat);
          addCell(tr, percent(row.Flat));
          addCell(tr, percent(sum));
          addCell(tr, row.CumFormat);
          addCell(tr, percent(row.Cum));
          addCell(tr, row.Name);
          addCell(tr, row.InlineLabel);
          fragment.appendChild(tr);
        }

        rows.textContent = ''; // Remove old rows
        rows.appendChild(fragment);
      }

      // Make different column headers trigger sorting.
      function bindSort(id, column) {
        const hdr = document.getElementById(id);
        if (hdr == null) return;
        const fn = function() { sortBy(column) };
        hdr.addEventListener('click', fn);
        hdr.addEventListener('touch', fn);
      }
      bindSort('flathdr1', 'Flat');
      bindSort('flathdr2', 'Flat');
      bindSort('cumhdr1', 'Cum');
      bindSort('cumhdr2', 'Cum');
      bindSort('namehdr', 'Name');
    }

    viewer(new URL(window.location.href), {{.Nodes}});
    makeTopTable({{.Total}}, {{.Top}});
  </script>
</body>
</html>
{{end}}

{{define "sourcelisting" -}}
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  {{template "css" .}}
  {{template "weblistcss" .}}
  {{template "weblistjs" .}}
</head>
<body>
  {{template "header" .}}
  <div id="content" class="source">
    {{.HTMLBody}}
  </div>
  {{template "script" .}}
  <script>viewer(new URL(window.location.href), null);</script>
</body>
</html>
{{end}}

{{define "plaintext" -}}
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  {{template "css" .}}
</head>
<body>
  {{template "header" .}}
  <div id="content">
    <pre>
      {{.TextBody}}
    </pre>
  </div>
  {{template "script" .}}
  <script>viewer(new URL(window.location.href), null);</script>
</body>
</html>
{{end}}

{{define "flamegraph" -}}
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  {{template "css" .}}
  <style type="text/css">{{template "d3flamegraphcss" .}}</style>
  <style type="text/css">
    .flamegraph-content {
      width: 90%;
      min-width: 80%;
      margin-left: 5%;
    }
    .flamegraph-details {
      height: 1.2em;
      width: 90%;
      min-width: 90%;
      margin-left: 5%;
      padding: 15px 0 35px;
    }
  </style>
</head>
<body>
  {{template "header" .}}
  <div id="bodycontainer">
    <div id="flamegraphdetails" class="flamegraph-details"></div>
    <div class="flamegraph-content">
      <div id="chart"></div>
    </div>
  </div>
  {{template "script" .}}
  <script>viewer(new URL(window.location.href), {{.Nodes}});</script>
  <script>{{template "d3script" .}}</script>
  <script>{{template "d3flamegraphscript" .}}</script>
  <script>
    var data = {{.FlameGraph}};

    var width = document.getElementById('chart').clientWidth;

    var flameGraph = d3.flamegraph()
      .width(width)
      .cellHeight(18)
      .minFrameSize(1)
      .transitionDuration(750)
      .transitionEase(d3.easeCubic)
      .inverted(true)
      .sort(true)
      .title('')
      .tooltip(false)
      .details(document.getElementById('flamegraphdetails'));

    // <full name> (percentage, value)
    flameGraph.label((d) => d.data.f + ' (' + d.data.p + ', ' + d.data.l + ')');

    (function(flameGraph) {
      var oldColorMapper = flameGraph.color();
      function colorMapper(d) {
        // Hack to force default color mapper to use 'warm' color scheme by not passing libtype
        const { data, highlight } = d;
        return oldColorMapper({ data: { n: data.n }, highlight });
      }

      flameGraph.color(colorMapper);
    }(flameGraph));

    d3.select('#chart')
      .datum(data)
      .call(flameGraph);

    function clear() {
      flameGraph.clear();
    }

    function resetZoom() {
      flameGraph.resetZoom();
    }

    window.addEventListener('resize', function() {
      var width = document.getElementById('chart').clientWidth;
      var graphs = document.getElementsByClassName('d3-flame-graph');
      if (graphs.length > 0) {
        graphs[0].setAttribute('width', width);
      }
      flameGraph.width(width);
      flameGraph.resetZoom();
    }, true);

    var search = document.getElementById('search');
    var searchAlarm = null;

    function selectMatching() {
      searchAlarm = null;

      if (search.value != '') {
        flameGraph.search(search.value);
      } else {
        flameGraph.clear();
      }
    }

    function handleSearch() {
      // Delay expensive processing so a flurry of key strokes is handled once.
      if (searchAlarm != null) {
        clearTimeout(searchAlarm);
      }
      searchAlarm = setTimeout(selectMatching, 300);
    }

    search.addEventListener('input', handleSearch);
  </script>
</body>
</html>
{{end}}
`))
}
