package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/adshmh/meter/db"
	"github.com/gojektech/heimdall/httpclient"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/pokt-foundation/portal-api-go/repository"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	// testRelayInterval = 10
	influxOptions = db.InfluxDBOptions{
		URL:           "http://localhost:8086",
		Token:         "mytoken",
		Org:           "myorg",
		DailyBucket:   "mainnetRelay",
		CurrentBucket: "mainnetRelay",
	}
)

func Test_RelayMeter_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	c := require.New(t)

	/* Initialize PHD Data */
	err := populatePocketHTTPDB()
	c.NoError(err)

	/* Create Test Influx Client */
	client := initTestInfluxClient(c)
	writeAPI := client.WriteAPI(influxOptions.Org, influxOptions.CurrentBucket)

	/* Create Test Client to Verify InfluxDB contents */
	testInfluxClient := db.NewInfluxDBSource(influxOptions)

	tests := []struct {
		name           string
		numberOfRelays int
		err            error
	}{
		{
			name:           "Should collect a set number of relays from Influx",
			numberOfRelays: 100_000,
			err:            nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			populateInfluxRelays(writeAPI, test.numberOfRelays)
			time.Sleep(1 * time.Second)

			timeTest := time.Now()

			// TEMP TEST VERIFICATION
			from, to := timeTest.AddDate(0, -2, 2), timeTest.AddDate(0, 0, 1)
			fmt.Println("DATES", "FROM", from, "TO", to)
			dailyCount, err := testInfluxClient.DailyCounts(from, to)
			c.NoError(err)

			PrettyString("DAILY COUNTS", dailyCount)

			// TODO - GETTING TODAYS COUNTS, NOT GETTING ANY OF TODAYS USAGE.
		})
	}

	client.Close()
}

func PrettyString(label string, thing interface{}) {
	jsonThing, _ := json.Marshal(thing)
	str := string(jsonThing)

	var prettyJSON bytes.Buffer
	_ = json.Indent(&prettyJSON, []byte(str), "", "    ")
	output := prettyJSON.String()

	fmt.Println(label, output)
}

/* Populate Influx DB */
func initTestInfluxClient(c *require.Assertions) influxdb2.Client {
	client := influxdb2.NewClientWithOptions(
		influxOptions.URL, influxOptions.Token,
		influxdb2.DefaultOptions().SetBatchSize(10_000),
	)

	health, err := client.Health(ctx)
	c.NoError(err)
	c.Equal("ready for queries and writes", *health.Message)

	bucketsAPI := client.BucketsAPI()
	bucket, err := bucketsAPI.FindBucketByName(ctx, influxOptions.DailyBucket)
	c.NoError(err)
	c.Equal(influxOptions.DailyBucket, bucket.Name)

	return client
}

// Sends a test batch of relays
func populateInfluxRelays(writeAPI api.WriteAPI, numberOfRelays int) {
	rand.Seed(time.Now().Unix())

	fmt.Println("CREATING RELAYS FOR", time.Now().UTC().AddDate(0, 0, -2))

	start := time.Now()
	for i := 0; i < numberOfRelays; i++ {
		randomIndex := rand.Intn(len(testRelays))
		randomRelay := testRelays[randomIndex]
		relayTimestamp := time.Now().UTC().AddDate(0, 0, -2)

		var result string
		if rand.Intn(63) <= 62 {
			result = "200"
		} else {
			result = "500"
		}

		relayPoint := influxdb2.NewPoint(
			"relay",
			map[string]string{
				"applicationPublicKey": randomRelay.applicationPublicKey,
				"nodePublicKey":        randomRelay.nodePublicKey,
				"method":               randomRelay.method,
				"result":               result,
				"blockchain":           randomRelay.blockchain,
				"blockchainSubdomain":  randomRelay.blockchainSubdomain,
				"host":                 "test_0bc93fa",
				"region":               "us-east-2",
			},
			map[string]interface{}{
				"bytes":       12345,
				"elapsedTime": randomRelay.elapsedTime,
				"count":       1,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(relayPoint)

		originPoint := influxdb2.NewPoint(
			"origin",
			map[string]string{},
			map[string]interface{}{
				"origin": randomRelay.origin,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(originPoint)
	}

	writeAPI.Flush()

	fmt.Printf("duration: %s", time.Since(start))
}

type RandomRelay struct {
	applicationPublicKey,
	nodePublicKey,
	method,
	blockchain,
	blockchainSubdomain,
	origin string
	elapsedTime float64
}

// Selection of relays to populate the test InfluxDB (randomly selected from this slice inside )
var testRelays = []RandomRelay{
	{
		applicationPublicKey: "test_efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c",
		nodePublicKey:        "test_node_pub_key_02fbcfbad0777942c1da5425bf0105546e7e7f53fffad9",
		method:               "eth_chainId",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.16475,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7",
		nodePublicKey:        "test_node_pub_key_29f42f262efa514fbe93af1e963b40c5cddb2200dd9e0a",
		method:               "eth_getBalance",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.20045,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c",
		nodePublicKey:        "test_node_pub_key_abfe693936a467d31e3246e32939ec04f5509315f39ef1",
		method:               "eth_getBlockByNumber",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.0813666666666667,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4",
		nodePublicKey:        "test_node_pub_key_277ed47d71a69e1aef94f8338f859af46d57840060f17d",
		method:               "eth_getCode",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.15785,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53",
		nodePublicKey:        "test_node_pub_key_f64fe10e37173c0e7117490291c11e81d8851175e7ad4d",
		method:               "eth_getTransactionCount",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.054675,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69",
		nodePublicKey:        "test_node_pub_key_498b2b95ba2db09a263314e5fe23ce94fbd555d23578c1",
		method:               "eth_getStorageAt",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.1093,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_5d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55",
		nodePublicKey:        "test_node_pub_key_66b14f98b70d7e77572a6c72db611ebe28cf75d9cf542f",
		method:               "eth_blockNumber",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.2205,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d",
		nodePublicKey:        "test_node_pub_key_b5c0fd8910bb7a121686865407e1a3ba40dcfb700d5e87",
		method:               "eth_getBalance",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.0932,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5",
		nodePublicKey:        "test_node_pub_key_124d8dcca024f05e01fb56c91800480926d62da44b0eee",
		method:               "eth_blockNumber",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.1162,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_46a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d",
		nodePublicKey:        "test_node_pub_key_dbd1105f79c1c40c2fc8d8749ea5760700cff6a0504016",
		method:               "eth_blockNumber",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.0814,
		origin:               "https://app.test1.io",
	},
}

/* Initialize Pocket HTTP DB */
const (
	phdBaseURL = "http://localhost:8090"
	apiKey     = "test_api_key_6789"
	// connectionString = "postgres://postgres:pgpassword@localhost:5432/postgres?sslmode=disable"
	testUserID = "test_user_id_0db3b6c631c4"
)

var (
	ErrResponseNotOK error = errors.New("Response not OK")
	testClient             = httpclient.NewClient(httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0))
)

func populatePocketHTTPDB() error {
	for i, application := range testRelays {
		/* Create Application -> POST /application */
		appInput := fmt.Sprintf(applicationJSON, i+1, testUserID, application.applicationPublicKey)
		createdApplication, err := postToPHD[repository.Application]("application", []byte(appInput))
		if err != nil {
			return err
		}

		/* Create Load Balancer -> POST /load_balancer */
		loadBalancerInput := fmt.Sprintf(loadBalancerJSON, i+1, testUserID, createdApplication.ID)
		_, err = postToPHD[repository.LoadBalancer]("load_balancer", []byte(loadBalancerInput))
		if err != nil {
			return err
		}
	}

	return nil
}

// Sends a POST request to PHD to populate the test Portal DB
func postToPHD[T any](path string, postData []byte) (T, error) {
	var data T

	rawURL := fmt.Sprintf("%s/%s", phdBaseURL, path)

	headers := http.Header{
		"Authorization": {apiKey},
		"Content-Type":  {"application/json"},
		"Connection":    {"Close"},
	}

	postBody := bytes.NewBufferString(string(postData))

	response, err := testClient.Post(rawURL, postBody, headers)
	if err != nil {
		return data, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return data, fmt.Errorf("%w. %s", ErrResponseNotOK, http.StatusText(response.StatusCode))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}
