package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"

	"github.com/broderickhyman/albiondata-api/lib"
	adslib "github.com/tikz/albiondata-sql/lib"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
)

var (
	version string
	cfgFile string
	db      *gorm.DB
)

var rootCmd = &cobra.Command{
	Use:   "albiondata-api",
	Short: "albiondata-api is the API Server for the Albion Data Project",
	Long: `Reads data from a SQL Database (MSSQL, MySQL, PostgreSQL and SQLite3 are supported), 
and serves them through a HTTP API`,
	Run: doCmd,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.albiondata-api.yaml")
	rootCmd.PersistentFlags().StringP("listen", "l", "[::1]:3080", "Host and port to listen on")
	rootCmd.PersistentFlags().StringP("dbType", "t", "mysql", "Database type must be one of mysql, postgresql, sqlite3")
	rootCmd.PersistentFlags().StringP("dbURI", "u", "", "Databse URI to connect to, see: http://jinzhu.me/gorm/database.html#connecting-to-a-database")
	rootCmd.PersistentFlags().IntP("minUpdatedAt", "m", 172800, "UpdatedAt must be >= now - this seconds")
	rootCmd.PersistentFlags().Bool("useHttps", false, "useHttps enables or disables AutoTLS")
	rootCmd.PersistentFlags().String("autoCertCacheDirectory", "", "Used when useHttps is true. Stores the TLS files in specified directory. https://echo.labstack.com/cookbook/auto-tls")
	rootCmd.PersistentFlags().String("staticFolderPath", "", "--DANGER-- Path to folder where static files reside for web server. https://echo.labstack.com/guide/static-files")
	rootCmd.PersistentFlags().String("staticFilePrefix", "", "--DANGER-- Prefix for static files to be served as, like example.com/prefix/index.html. https://echo.labstack.com/guide/static-files")
	viper.BindPFlag("listen", rootCmd.PersistentFlags().Lookup("listen"))
	viper.BindPFlag("dbType", rootCmd.PersistentFlags().Lookup("dbType"))
	viper.BindPFlag("dbURI", rootCmd.PersistentFlags().Lookup("dbURI"))
	viper.BindPFlag("minUpdatedAt", rootCmd.PersistentFlags().Lookup("minUpdatedAt"))
	viper.BindPFlag("useHttps", rootCmd.PersistentFlags().Lookup("useHttps"))
	viper.BindPFlag("autoCertCacheDirectory", rootCmd.PersistentFlags().Lookup("autoCertCacheDirectory"))
	viper.BindPFlag("staticFolderPath", rootCmd.PersistentFlags().Lookup("staticFolderPath"))
	viper.BindPFlag("staticFilePrefix", rootCmd.PersistentFlags().Lookup("staticFilePrefix"))
}

func initConfig() {
	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("/etc")

		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName("albiondata-api")

		// Add the executable path as
		ex, err := os.Executable()
		if err != nil {
			panic(err)
		}
		exPath := filepath.Dir(ex)
		viper.AddConfigPath(exPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
	}

	viper.SetEnvPrefix("ADA")
	viper.AutomaticEnv()
}

func apiHandleStatsPricesItemJson(c echo.Context) error {
	return c.JSON(http.StatusOK, getStatsPricesItem(c))
}

func apiHandleStatsPricesView(c echo.Context) error {
	results := getStatsPricesItem(c)

	html :=
		`<html>
	<head>
		<style>
			table, th, td {
				border: 1px solid black;
				border-collapse: collapse;
			}
		</style>
	</head>
	<body>
		<table style='width:100%'>
			<tr>
				<th>item_id</th>
				<th>city</th>
				<th>sell_price_min</th>
				<th>sell_price_min_date</th>
				<th>sell_price_max</th>
				<th>sell_price_max_date</th>
				<th>buy_price_min</th>
				<th>buy_price_min_date</th>
				<th>buy_price_max</th>
				<th>buy_price_max_date</th>
			</tr>`
	for _, result := range results {
		html += "<tr>"
		v := reflect.ValueOf(result)

		for i := 0; i < v.NumField(); i++ {
			html += fmt.Sprintf("<td>%v</td>", v.Field(i).Interface())
		}
		html += "</tr>"
	}

	html +=
		`		</table>
	</body>
</html>`

	return c.HTML(http.StatusOK, html)
}

func getStatsPricesItem(c echo.Context) []lib.APIStatsPricesItem {
	result := []lib.APIStatsPricesItem{}

	minimumAge := 172800
	if viper.IsSet("minUpdatedAt") {
		minimumAge = viper.GetInt("minUpdatedAt")
	}
	// age query param
	ageInt, err := strconv.Atoi(c.QueryParam("age"))
	if err == nil && ageInt < minimumAge {
		minimumAge = ageInt
	}
	ageTime := time.Now().Add(-time.Duration(minimumAge) * time.Second)

	// location query param
	locs := adslib.Locations()
	if len(c.QueryParam("locations")) > 0 {
		queryLocs := strings.Split(c.QueryParam("locations"), ",")

		locs = []adslib.Location{}
		for _, queryLoc := range queryLocs {
			for _, l := range adslib.Locations() {
				if strings.Contains(l.String(), queryLoc) {
					locs = append(locs, l)
					break
				}
			}
		}
	}

	// item query param
	queryItemIDs := strings.Split(c.Param("item"), ",")
	itemIDs := []string{}

	for _, qID := range queryItemIDs {
		if qID == "*" {
			continue
		}
		if strings.Contains(qID, "*") {
			sqlID := strings.Replace(qID, "*", "%", -1)

			foundIDs := []string{}
			if err := db.Table(adslib.NewModelMarketOrder().TableName()).Select("item_id").Where("item_id LIKE ? and updated_at >= ?", sqlID, ageTime).Group("item_id").Pluck("item_id", &foundIDs).Error; err != nil {
				fmt.Printf("%v\n", err)
				continue
			}

			itemIDs = append(itemIDs, foundIDs...)

		} else {
			itemIDs = append(itemIDs, qID)
		}
	}

	for _, itemID := range itemIDs {
		for _, l := range locs {
			lres := lib.APIStatsPricesItem{
				ItemID: itemID,
				City:   l.String(),
			}

			found := false

			// Find lowest offer price
			m := adslib.NewModelMarketOrder()
			if err := db.Select("*, strftime('%Y-%m-%d %H:%i', `updated_at`) as updated_at_no_seconds").Where("location = ? and item_id = ? and auction_type = ? and updated_at >= ?", l, itemID, "offer", ageTime).Order("updated_at_no_seconds desc, price asc").First(&m).Error; err == nil {
				found = true
				lres.SellPriceMin = m.Price
				lres.SellPriceMinDate = m.UpdatedAt
			}

			// Find highest offer price
			m = adslib.NewModelMarketOrder()
			if err := db.Select("*, strftime('%Y-%m-%d %H:%i', `updated_at`) as updated_at_no_seconds").Where("location = ? and item_id = ? and auction_type = ? and updated_at >= ?", l, itemID, "offer", ageTime).Order("updated_at_no_seconds desc, price desc").First(&m).Error; err == nil {
				found = true
				lres.SellPriceMax = m.Price
				lres.SellPriceMaxDate = m.UpdatedAt
			}

			// Find lowest request price
			m = adslib.NewModelMarketOrder()
			if err := db.Select("*, strftime('%Y-%m-%d %H:%i', `updated_at`) as updated_at_no_seconds").Where("location = ? and item_id = ? and auction_type = ? and updated_at >= ?", l, itemID, "request", ageTime).Order("updated_at_no_seconds desc, price asc").First(&m).Error; err == nil {
				found = true
				lres.BuyPriceMin = m.Price
				lres.BuyPriceMinDate = m.UpdatedAt
			}

			// Find highest request price
			m = adslib.NewModelMarketOrder()
			if err := db.Select("*, strftime('%Y-%m-%d %H:%i', `updated_at`) as updated_at_no_seconds").Where("location = ? and item_id = ? and auction_type = ? and updated_at >= ?", l, itemID, "request", ageTime).Order("updated_at_no_seconds desc, price desc").First(&m).Error; err == nil {
				found = true
				lres.BuyPriceMax = m.Price
				lres.BuyPriceMaxDate = m.UpdatedAt
			}

			if found {
				result = append(result, lres)
			}
		}
	}
	return result
}

func apiHandleStatsChartsItem(c echo.Context) error {
	result := []lib.APIStatsChartsResponse{}

	// location query param
	locs := adslib.Locations()
	if len(c.QueryParam("locations")) > 0 {
		queryLocs := strings.Split(c.QueryParam("locations"), ",")

		locs = []adslib.Location{}
		for _, queryLoc := range queryLocs {
			for _, l := range adslib.Locations() {
				if strings.Contains(l.String(), queryLoc) {
					locs = append(locs, l)
					break
				}
			}
		}
	}

	item := c.Param("item")

	dbResults := []adslib.ModelMarketStats{}

	for _, l := range locs {
		lResult := lib.APIStatsChartsLocationResponse{}

		db.Where("item_id = ? AND location = ?", item, l).Find(&dbResults)

		if len(dbResults) > 0 {
			for _, dbResult := range dbResults {
				lResult.Timestamps = append(lResult.Timestamps, dbResult.Timestamp.Unix()*1000) // *1000 For charts.js which wants milliseconds
				lResult.PricesMin = append(lResult.PricesMin, dbResult.PriceMin)
				lResult.PricesMax = append(lResult.PricesMax, dbResult.PriceMax)
				lResult.PricesAvg = append(lResult.PricesAvg, dbResult.PriceAvg)
			}

			result = append(result, lib.APIStatsChartsResponse{
				Location: l.String(),
				Data:     lResult,
			})
		}
	}

	return c.JSON(http.StatusOK, result)
}

func apiHandleStatsGold(c echo.Context) error {
	result := lib.APIStatesChartsResponse{}

	dbResults := []adslib.ModelGoldprices{}
	db.Find(&dbResults)

	for _, dbResult := range dbResults {
		result.Timestamps = append(result.Timestamps, dbResult.Timestamp.Unix()*1000)
		result.Prices = append(result.Prices, dbResult.Price)
	}

	return c.JSON(http.StatusOK, result)
}

func doCmd(cmd *cobra.Command, args []string) {
	//******************************
	// START DB
	fmt.Printf("Connecting to database: %s\n", viper.GetString("dbType"))
	var err error
	db, err = gorm.Open(viper.GetString("dbType"), viper.GetString("dbURI"))
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}

	// Debug
	db.LogMode(true)

	defer db.Close()
	// END DB
	//******************************

	//******************************
	// START ECHO
	e := echo.New()
	e.HideBanner = true

	// Cache certificates
	if viper.GetBool("useHttps") {
		e.Pre(middleware.HTTPSWWWRedirect())
		if viper.GetString("autoCertCacheDirectory") != "" {
			e.AutoTLSManager.Cache = autocert.DirCache(viper.GetString("autoCertCacheDirectory"))
		}
	}

	// Recover from panics
	e.Use(middleware.Recover())

	// Logger
	e.Use(middleware.Logger())

	//Allow CORS
	e.Use(middleware.CORS())

	if viper.GetString("staticFilePrefix") != "" && viper.GetString("staticFolderPath") != "" {
		e.Static(viper.GetString("staticFilePrefix"), viper.GetString("staticFolderPath"))
	} else {
		e.GET("/", func(c echo.Context) error {
			return c.Redirect(http.StatusTemporaryRedirect, "https://www.albion-online-data.com")
		})
	}

	e.GET("/api/v1/stats/prices/:item", apiHandleStatsPricesItemJson)
	e.GET("/api/v1/stats/charts/:item", apiHandleStatsChartsItem)
	e.GET("/api/v1/stats/view/:item", apiHandleStatsPricesView)
	e.GET("/api/v1/stats/gold", apiHandleStatsGold)

	// Start server
	if viper.GetBool("useHttps") {
		go func(c *echo.Echo) {
			e.Logger.Fatal(e.Start(":80"))
		}(e)
		e.Logger.Fatal(e.StartAutoTLS(viper.GetString("listen")))
	} else {
		e.Logger.Fatal(e.Start(viper.GetString("listen")))
	}

	// END ECHO
	//*******************************
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
