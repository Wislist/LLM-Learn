// Command sample_data generates a sample sales xlsx so the chart path can be
// tested without real company data.
//
//	go run ./scripts/sample_data
//
// Writes ./data/sample_sales.xlsx with 3 products × 18 months of random-ish rows.
package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/xuri/excelize/v2"
)

func main() {
	out := "./data/sample_sales.xlsx"
	_ = os.MkdirAll(filepath.Dir(out), 0o755)

	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	headers := []string{"order_date", "product", "region", "qty", "amount"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, cell(1, i+1), h)
	}
	products := []string{"通风系统", "配电柜", "监控设备"}
	regions := []string{"华东", "华北", "华南", "西部"}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	row := 2
	for m := 0; m < 18; m++ {
		date := start.AddDate(0, m, 0)
		for _, p := range products {
			for _, r := range regions {
				qty := 40 + rng.Intn(120)
				price := 1500 + rng.Float64()*4000
				_ = f.SetCellValue(sheet, cell(row, 1), date.Format("2006-01-02"))
				_ = f.SetCellValue(sheet, cell(row, 2), p)
				_ = f.SetCellValue(sheet, cell(row, 3), r)
				_ = f.SetCellValue(sheet, cell(row, 4), qty)
				_ = f.SetCellValue(sheet, cell(row, 5), fmt.Sprintf("%.2f", float64(qty)*price))
				row++
			}
		}
	}
	if err := f.SaveAs(out); err != nil {
		fmt.Println("save:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d rows)\n", out, row-2)
}

func cell(row, col int) string {
	return fmt.Sprintf("%c%d", 'A'+col-1, row)
}
