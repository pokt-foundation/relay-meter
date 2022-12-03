package tests

const (
	applicationJSON = `{
		"name": "test-application-%d",
		"userID": "%s",
		"status": "IN_SERVICE",
		"dummy": true,
		"firstDateSurpassed": null,
		"limit": {
			"payPlan": {
				"planType": "FREETIER_V0",
				"dailyLimit": 250000
			}
		},
		"gatewayAAT": {
			"address": "test_address_8dbb89278918da056f589086fb4",
			"applicationPublicKey": "%s",
			"applicationSignature": "test_key_f9c21a35787c53c8cdb532cad0dc01e099f34c28219528e3732c2da38037a84db4ce0282fa9aa404e56248155a1fbda789c8b5976711ada8588ead5",
			"clientPublicKey": "test_key_2381d403a2e2edeb37c284957fb0ee5d66f1081acb87478a5817919",
			"privateKey": "test_key_0c0fbd26d98bcbdca4d4f14fdc45ffb6db0e6a23a56671fc4b444e1b8bbd4a934adde984d117f281867cb71d9537de45473b3028ead2326cd9e27ad",
			"version": "0.0.1"
		},
		"gatewaySettings": {
			"secretKey": "test_key_ba2724be652eca0a350bc07",
			"secretKeyRequired": false
		},
			"notificationSettings": {
			"signedUp": true,
			"quarter": false,
			"half": false,
			"threeQuarters": true,
			"full": true
		}
	}`

	loadBalancerJSON = `{
		"name": "test-load-balancer-%d",
		"userID": "%s",
		"applicationIDs": ["%s"],
		"requestTimeout": 2000,
		"gigastake": false,
		"gigastakeRedirect": true,
		"stickinessOptions": {
			"duration": "",
			"stickyOrigins": null,
			"stickyMax": 0,
			"stickiness": false
		},
		"Applications": null
	}`
)
