package cms

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/hilmimuharromi/voxin-portal/internal/store"
)

type AnalyticsVM struct {
	Days       int
	Views      int
	Uniques    int
	Published  int
	Chart      template.HTML
	Daily      []store.DayStat
	Sources    []store.NamedStat
	Regions    []store.NamedStat
	Categories []store.NamedStat
	Top        []store.Article
}

var sourceLabels = map[string]string{"search": "Mesin pencari", "sosmed": "Media sosial", "langsung": "Langsung", "lainnya": "Situs lain", "internal": "Dari halaman voxin.id"}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	site, _ := s.App.Site(ctx)
	days, _ := strconv.Atoi(r.URL.Query().Get("hari"))
	if days != 7 && days != 28 && days != 90 {
		days = 28
	}
	vm := AnalyticsVM{Days: days}
	var err error
	if vm.Daily, err = s.App.Store.DailyStats(ctx, site.ID, days); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for _, d := range vm.Daily {
		vm.Views += d.Views
		vm.Uniques += d.Uniques
	}
	vm.Chart = barChart(vm.Daily)
	vm.Sources, _ = s.App.Store.SourceStats(ctx, site.ID, days)
	for i := range vm.Sources {
		if l := sourceLabels[vm.Sources[i].Key]; l != "" {
			vm.Sources[i].Name = l
		}
	}
	vm.Regions, _ = s.App.Store.RegionStats(ctx, site.ID, days)
	vm.Categories, _ = s.App.Store.CategoryStats(ctx, site.ID, days)
	vm.Top, _ = s.App.Store.TopArticles(ctx, site.ID, days, 15)
	_, vm.Published, _ = s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Limit: 1})
	s.render(w, r, "analytics", "Analitik", "analytics", vm)
}

// barChart draws daily views as one-series bars (inline SVG, no JS).
// Each bar has a wide invisible hit area with a native tooltip.
func barChart(days []store.DayStat) template.HTML {
	if len(days) == 0 {
		return ""
	}
	const W, H, padL, padB, padT = 720, 220, 44, 24, 12
	maxV := 0
	for _, d := range days {
		if d.Views > maxV {
			maxV = d.Views
		}
	}
	top := niceMax(maxV)
	plotW, plotH := float64(W-padL), float64(H-padB-padT)
	step := plotW / float64(len(days))
	bw := step * 0.62
	if bw > 22 {
		bw = 22
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="Tayangan harian, %d hari terakhir">`, W, H, len(days))
	for i := 0; i <= 4; i++ {
		v := top * i / 4
		y := float64(padT) + plotH - plotH*float64(i)/4
		fmt.Fprintf(&b, `<line x1="%d" x2="%d" y1="%.1f" y2="%.1f" class="grid"/><text x="%d" y="%.1f" class="ax" text-anchor="end">%s</text>`,
			padL, W, y, y, padL-8, y+4, fmtNum(v))
	}
	labelEvery := 1
	if len(days) > 10 {
		labelEvery = len(days) / 7
	}
	for i, d := range days {
		x := float64(padL) + step*float64(i) + (step-bw)/2
		h := 0.0
		if top > 0 {
			h = plotH * float64(d.Views) / float64(top)
		}
		y := float64(padT) + plotH - h
		label := fmt.Sprintf("%d %s", d.Day.Day(), bulanPendek[d.Day.Month()-1])
		if h > 0 {
			r := 4.0
			if h < r {
				r = h / 2
			}
			// rounded top, square base anchored to the baseline
			fmt.Fprintf(&b, `<path class="bar" d="M%.1f %.1f V%.1f Q%.1f %.1f %.1f %.1f H%.1f Q%.1f %.1f %.1f %.1f V%.1f Z"/>`,
				x, y+h, y+r, x, y, x+r, y, x+bw-r, x+bw, y, x+bw, y+r, y+h)
		}
		fmt.Fprintf(&b, `<rect class="hit" x="%.1f" y="%d" width="%.1f" height="%.1f"><title>%s: %s tayangan, %s pembaca unik</title></rect>`,
			float64(padL)+step*float64(i), padT, step, plotH, label, fmtNum(d.Views), fmtNum(d.Uniques))
		if i%labelEvery == 0 || i == len(days)-1 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%d" class="ax" text-anchor="middle">%s</text>`, x+bw/2, H-6, label)
		}
	}
	fmt.Fprintf(&b, `<line x1="%d" x2="%d" y1="%.1f" y2="%.1f" class="base"/></svg>`, padL, W, float64(padT)+plotH, float64(padT)+plotH)
	return template.HTML(b.String())
}

var bulanPendek = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func niceMax(v int) int {
	if v <= 4 {
		return 4
	}
	for _, step := range []int{5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000, 10000, 20000, 25000, 50000, 100000} {
		if v <= step*4 {
			return step * 4
		}
	}
	return v
}
