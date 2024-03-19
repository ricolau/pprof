package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/pprof/internal/binutils"
	"github.com/google/pprof/internal/graph"
	"github.com/google/pprof/internal/plugin"
	"github.com/google/pprof/internal/report"
	"github.com/google/pprof/internal/transport"
	"github.com/google/pprof/profile"
	"html/template"
	"net/http"
	"os"
)

var singleTransport http.RoundTripper

type renderData map[string]string

type RenderOption struct {
	DiffType     string
	BaseFilePath string
}

func GetRenderFunc(filepath string, renderType string, renderData UdfRenderData, ro RenderOption) (func(w http.ResponseWriter, req *http.Request), error) {

	if singleTransport == nil {
		o := &plugin.Options{}
		o.Flagset = &GoFlags{}
		singleTransport = transport.New(o.Flagset)
	}

	cxt := context.Background()
	o := &plugin.Options{}
	o.Flagset = &GoFlags{}
	o.HTTPTransport = singleTransport

	o = setDefaults(o)
	//src, cmd, err := initSource(cxt, filepath, o)
	src, _, _ := initSource(cxt, filepath, o, ro)
	p, err := fetchProfiles(src, o)
	copier := makeProfileCopier(p)
	ui, err := makeWebInterface2(p, copier, o)

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
	ui.help["flamegraph"] = "Display profile as a flame graph"
	ui.help["reset"] = "Show the entire profile"
	ui.help["save_config"] = "Save current settings"

	ui.renderData = renderData

	switch renderType {
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

func initRenderArgs(rd UdfRenderData) webArgs2 {
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

	fmt.Println(wd.Peekurl, 1111)
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
	ui.render(w, req, "top", rpt, errList, legend, wd)
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
	ui.render(w, req, "plaintext", rpt, errList, legend, wd)

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
	ui.render(w, req, "sourcelisting", rpt, errList, legend, wd)
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
	ui.render(w, req, "plaintext", rpt, errList, legend, wd)

}

// flamegraph generates a web page containing a flamegraph.
func (ui *webInterface2) flamegraph(w http.ResponseWriter, req *http.Request) {
	// Get all data in a report.
	rpt, errList := ui.makeReport(w, req, []string{"svg"}, func(cfg *config) {
		cfg.CallTree = true
		cfg.Trim = false
		cfg.Granularity = "filefunctions"
	})
	if rpt == nil {
		return // error already reported
	}

	// Make stack data and generate corresponding JSON.
	stacks := rpt.Stacks()
	b, err := json.Marshal(stacks)
	if err != nil {
		http.Error(w, "error serializing stacks for flame graph",
			http.StatusInternalServerError)
		ui.options.UI.PrintErr(err)
		return
	}

	nodes := make([]string, len(stacks.Sources))
	for i, src := range stacks.Sources {
		nodes[i] = src.FullName
	}
	nodes[0] = "" // root is not a real node

	_, legend := report.TextItems(rpt)
	wd := initRenderArgs(ui.renderData)
	wd.Stacks = template.JS(b)
	wd.Nodes = nodes
	ui.render(w, req, "flamegraph", rpt, errList, legend, wd)

}

type UdfRenderData struct {
	Topurl        string
	Graphurl      string
	Flamegraphurl string
	Peekurl       string
	Sourceurl     string
	Disasmurl     string
	Sampleurl     string
	Cpuurl        string
	Downloadurl   string
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
	Stacks      template.JS
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
	copier       profileCopier
	options      *plugin.Options
	help         map[string]string
	templates    *template.Template
	settingsFile string
	renderData   UdfRenderData
}

func makeWebInterface2(p *profile.Profile, copier profileCopier, opt *plugin.Options) (*webInterface2, error) {
	settingsFile, err := settingsFileName()
	if err != nil {
		return nil, err
	}
	templates := template.New("templategroup")
	addTemplates2(templates)
	report.AddSourceTemplates(templates)
	return &webInterface2{
		prof:         p,
		copier:       copier,
		options:      opt,
		help:         make(map[string]string),
		templates:    templates,
		settingsFile: settingsFile,
	}, nil
}

func initSource(c context.Context, filepath string, o *plugin.Options, ro RenderOption) (*source, []string, error) {
	//flag := o.Flagset
	// Comparisons.

	ds1 := []*string{}
	if ro.DiffType == "diff-base" && ro.BaseFilePath != "" {
		ds1 = append(ds1, &ro.BaseFilePath)
	}
	flagDiffBase := &ds1

	ds2 := []*string{}
	if ro.DiffType == "base" && ro.BaseFilePath != "" {
		ds2 = append(ds2, &ro.BaseFilePath)
	}
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
	ei2 := -1
	flagSeconds := &ei2
	// Heap profile options

	df4 := false
	flagInUseSpace := &df4

	df5 := false
	flagInUseObjects := &df5

	df6 := false
	flagAllocSpace := &df6

	df7 := false
	flagAllocObjects := &df7
	// Contention profile options

	defaultFalse := false
	flagTotalDelay := &defaultFalse

	df3 := false
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
	_, rpt, err := generateRawReport(ui.copier.newCopy(), cmd, cfg, &options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		ui.options.UI.PrintErr(err)
		return nil, nil
	}
	return rpt, catcher.errors
}

// addTemplates2 adds a set of template definitions to templates.
func addTemplates2(templates *template.Template) {
	// Load specified file.
	loadFile := func(fname string) string {
		data, err := embeddedFiles.ReadFile(fname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "internal/driver: embedded file %q not found\n",
				fname)
			os.Exit(1)
		}
		return string(data)
	}
	loadCSS := func(fname string) string {
		return `<style type="text/css">` + "\n" + loadFile(fname) + `</style>` + "\n"
	}
	loadJS := func(fname string) string {
		return `<script>` + "\n" + loadFile(fname) + `</script>` + "\n"
	}

	// Define a named template with specified contents.
	def := func(name, contents string) {
		sub := template.New(name)
		template.Must(sub.Parse(contents))
		template.Must(templates.AddParseTree(name, sub.Tree))
	}

	// Embedded files.
	def("css", loadCSS("html/common.css"))
	def("header", loadFile("html/header.html"))
	def("graph", loadFile("html/graph.html"))
	def("script", loadJS("html/common.js"))
	def("top", loadFile("html/top.html"))
	def("sourcelisting", loadFile("html/source.html"))
	def("plaintext", loadFile("html/plaintext.html"))
	// TODO: Rename "stacks" to "flamegraph" to seal moving off d3 flamegraph.
	def("stacks", loadFile("html/stacks.html"))
	def("stacks_css", loadCSS("html/stacks.css"))
	def("stacks_js", loadJS("html/stacks.js"))
}
