package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime/pprof"
	"sort"
	"strings"
	"text/tabwriter"
)

const version = `0.1.1`

var languages = []language{
	language{"Thrift", mExt(".thrift"), cComments},

	language{"C", mExt(".c", ".h"), cComments},
	language{"C++", mExt(".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx"), cComments},
	language{"Go", mExt(".go"), cComments},
	language{"Scala", mExt(".scala"), cComments},
	language{"Java", mExt(".java"), cComments},
	language{"Dart", mExt(".dart"), noComments},

	language{"YACC", mExt(".y"), cComments},
	language{"Lex", mExt(".l"), cComments},

	language{"SQL", mExt(".sql"), sqlComments},

	language{"Haskell", mExt(".hs", ".lhs"), hsComments},

	language{"Perl", mExt(".pl", ".pm"), shComments},
	language{"PHP", mExt(".php"), cComments},
	language{"Pascal", mExt(".pas", ".dpr", ".inc"), pasComments},

	language{"Shell", mExt(".sh"), shComments},
	language{"Bash", mExt(".bash"), shComments},

	language{"Ruby", mExt(".rb"), shComments},
	language{"Python", mExt(".py"), pyComments},
	language{"Assembly", mExt(".asm", ".s"), semiComments},
	language{"Lisp", mExt(".lsp", ".lisp"), semiComments},
	language{"Scheme", mExt(".scm", ".scheme"), semiComments},

	language{"Make", mName("makefile", "Makefile", "MAKEFILE"), shComments},
	language{"CMake", mName("CMakeLists.txt"), shComments},
	language{"Jam", mName("Jamfile", "Jamrules"), shComments},

	language{"Markdown", mExt(".md"), noComments},

	language{"HAML", mExt(".haml"), noComments},
	language{"SASS", mExt(".sass"), cssComments},
	language{"SCSS", mExt(".scss"), cssComments},

	language{"HTML", mExt(".htm", ".html", ".xhtml"), xmlComments},
	language{"XML", mExt(".xml"), xmlComments},
	language{"CSS", mExt(".css"), cssComments},
	language{"JavaScript", mExt(".js"), cComments},
	language{"TypeScript", mExt(".ts", ".tsx"), cComments},
	language{"JSON", mExt(".json"), noComments},
}

type commenter struct {
	LineComment  string
	StartComment string
	EndComment   string
	Nesting      bool
}

var (
	noComments   = commenter{"\000", "\000", "\000", false}
	xmlComments  = commenter{"\000", `<!--`, `-->`, false}
	cComments    = commenter{`//`, `/*`, `*/`, false}
	cssComments  = commenter{"\000", `/*`, `*/`, false}
	shComments   = commenter{`#`, "\000", "\000", false}
	semiComments = commenter{`;`, "\000", "\000", false}
	hsComments   = commenter{`--`, `{-`, `-}`, true}
	sqlComments  = commenter{`--`, "\000", "\000", false}
	pyComments   = commenter{`#`, `"""`, `"""`, false}
	pasComments  = commenter{`//`, `{`, `}`, false}
)

type language struct {
	namer
	matcher
	commenter
}

// TODO work properly with unicode
func (l language) Update(c []byte, s *stats) {
	s.FileCount++

	inComment := 0 // this is an int for nesting
	inLComment := false
	blank := true
	lc := []byte(l.LineComment)
	sc := []byte(l.StartComment)
	ec := []byte(l.EndComment)
	lp, sp, ep := 0, 0, 0

	for _, b := range c {
		if inComment == 0 && b == lc[lp] {
			lp++
			if lp == len(lc) {
				inLComment = true
				lp = 0
			}
		} else {
			lp = 0
		}
		if !inLComment && b == sc[sp] {
			sp++
			if sp == len(sc) {
				inComment++
				if inComment > 1 && !l.Nesting {
					inComment = 1
				}
				sp = 0
			}
		} else {
			sp = 0
		}
		if !inLComment && inComment > 0 && b == ec[ep] {
			ep++
			if ep == len(ec) {
				if inComment > 0 {
					inComment--
				}
				ep = 0
			}
		} else {
			ep = 0
		}

		if b != byte(' ') && b != byte('\t') && b != byte('\n') && b != byte('\r') {
			blank = false
		}

		// BUG(srl): lines with comment don't count towards code
		// Note that lines with both code and comment count towards
		// each, but are not counted twice in the total.
		if b == byte('\n') {
			s.TotalLines++
			if inComment > 0 || inLComment {
				inLComment = false
				s.CommentLines++
			} else if blank {
				s.BlankLines++
			} else {
				s.CodeLines++
			}
			blank = true
			continue
		}
	}
}

type namer string

func (l namer) Name() string { return string(l) }

type matcher func(string) bool

func (m matcher) Match(fname string) bool { return m(fname) }

func mExt(exts ...string) matcher {
	return func(fname string) bool {
		for _, ext := range exts {
			if ext == path.Ext(fname) {
				return true
			}
		}
		return false
	}
}

func mName(names ...string) matcher {
	return func(fname string) bool {
		for _, name := range names {
			if name == path.Base(fname) {
				return true
			}
		}
		return false
	}
}

type stats struct {
	FileCount    int
	TotalLines   int
	CodeLines    int
	BlankLines   int
	CommentLines int
}

var info = map[string]*stats{}

func handleFile(fname string) {
	var l language
	ok := false
	for _, lang := range languages {
		if lang.Match(fname) {
			ok = true
			l = lang
			break
		}
	}
	if !ok {
		return // ignore this file
	}
	i, ok := info[l.Name()]
	if !ok {
		i = &stats{}
		info[l.Name()] = i
	}
	c, err := ioutil.ReadFile(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ! %s\n", fname)
		return
	}
	l.Update(c, i)
}

var files []string

func add(n string) {
	fi, err := os.Stat(n)
	if err != nil {
		goto invalid
	}
	if fi.IsDir() {
		if len(ignoreDirs) > 0 {
			for _, d := range ignoreDirs {
				if fi.Name() == d {
					return
				}
			}
		}
		fs, err := ioutil.ReadDir(n)
		if err != nil {
			goto invalid
		}
		for _, f := range fs {
			if f.Name()[0] != '.' {
				add(path.Join(n, f.Name()))
			}
		}
		return
	}
	if fi.Mode()&os.ModeType == 0 {
		files = append(files, n)
		return
	}

	println(fi.Mode())

invalid:
	fmt.Fprintf(os.Stderr, "  ! %s\n", n)
}

type ldata []lresult

func (d ldata) Len() int { return len(d) }

func (d ldata) Less(i, j int) bool {
	if d[i].CodeLines == d[j].CodeLines {
		return d[i].Name > d[j].Name
	}
	return d[i].CodeLines > d[j].CodeLines
}

func (d ldata) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

type lresult struct {
	Name         string
	FileCount    int
	CodeLines    int
	CommentLines int
	BlankLines   int
	TotalLines   int
}

func (r *lresult) Add(a lresult) {
	r.FileCount += a.FileCount
	r.CodeLines += a.CodeLines
	r.CommentLines += a.CommentLines
	r.BlankLines += a.BlankLines
	r.TotalLines += a.TotalLines
}

func printJSON() {
	bs, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bs))
}

func printInfo() {
	w := tabwriter.NewWriter(os.Stdout, 2, 8, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "Language\tFiles\tCode\tComment\tBlank\tTotal\t")
	d := ldata([]lresult{})
	total := &lresult{}
	total.Name = "Total"
	for n, i := range info {
		r := lresult{
			n,
			i.FileCount,
			i.CodeLines,
			i.CommentLines,
			i.BlankLines,
			i.TotalLines,
		}
		d = append(d, r)
		total.Add(r)
	}
	d = append(d, *total)
	sort.Sort(d)
	//d[0].Name = "Total"
	for _, i := range d {
		fmt.Fprintf(
			w,
			"%s\t%d\t%d\t%d\t%d\t%d\t\n",
			i.Name,
			i.FileCount,
			i.CodeLines,
			i.CommentLines,
			i.BlankLines,
			i.TotalLines)
	}

	w.Flush()
}

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	useJSON    = flag.Bool("json", false, "JSON-format output")
	v          = flag.Bool("V", false, "display version info and exit")
	dirs       = flag.String("ignore", "", `ignore directory names i.e -ignore "dist,node_modules,vendor"`)
	ignoreDirs []string
)

func main() {
	flag.Parse()
	if *v {
		fmt.Printf("sloc %s\n", version)
		return
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			return
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	args := flag.Args()
	if len(args) == 0 {
		args = append(args, `.`)
	}

	if len(*dirs) > 0 {
		ignoreDirs = strings.Split(*dirs, ",")
	}

	for _, n := range args {
		add(n)
	}

	for _, f := range files {
		handleFile(f)
	}

	if *useJSON {
		printJSON()
	} else {
		printInfo()
	}
}
