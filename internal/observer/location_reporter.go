package observer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/free5gc/amf/internal/context"
	"github.com/free5gc/openapi/models"
)

var lastLocations = make(map[string]string)
var mu sync.Mutex

func StartLocationReporter() {
	go func() {
		for {
			checkAndReport()
			time.Sleep(2 * time.Second) // 每2秒检测一次
		}
	}()
}

func checkAndReport() {
	amfSelf := context.GetSelf()
	fmt.Println("==== EventSubscriptions Dump ====")
	amfSelf.EventSubscriptions.Range(func(key, value interface{}) bool {
		sub := value.(*context.AMFContextEventSubscription)
		fmt.Printf("SubscriptionId: %v\n", key)
		fmt.Printf("  NotifyUri: %v\n", sub.EventSubscription.EventNotifyUri)
		fmt.Printf("  EventList: %+v\n", sub.EventSubscription.EventList)
		fmt.Printf("  UeSupiList: %+v\n", sub.UeSupiList)
		fmt.Printf("  IsAnyUe: %v\n", sub.IsAnyUe)
		return true
	})
	fmt.Println("==== End Dump ====")
	amfSelf.UePool.Range(func(key, value interface{}) bool {
		ue := value.(*context.AmfUe)
		supi := ue.Supi
		locationBytes, _ := json.Marshal(ue.Location)
		locationStr := string(locationBytes)

		fmt.Printf("[DEBUG] check UE: supi=%s, location=%s\n", supi, locationStr)

		mu.Lock()
		lastLocations[supi] = locationStr // 强制每次都推送
		mu.Unlock()
		sendLocationReport(ue)
		return true
	})
}

func sendLocationReport(ue *context.AmfUe) {
	fmt.Printf("[DEBUG] sendLocationReport called for supi=%s\n", ue.Supi)
	amfSelf := context.GetSelf()
	amfSelf.EventSubscriptions.Range(func(key, value interface{}) bool {
		sub := value.(*context.AMFContextEventSubscription)
		// 只要是anyUE，直接推送
		if sub.IsAnyUe {
			for _, event := range sub.EventSubscription.EventList {
				fmt.Printf("[DEBUG] event.Type=%v, expect=%v\n", event.Type, models.AmfEventType_LOCATION_REPORT)
				if event.Type == "LOCATION_REPORT" || event.Type == models.AmfEventType_LOCATION_REPORT {
					report := makeLocationReport(ue, key.(string))
					fmt.Printf("[DEBUG] Ready to post LOCATION_REPORT for supi=%s to %s\n", ue.Supi, sub.EventSubscription.EventNotifyUri)
					go postToUri(sub.EventSubscription.EventNotifyUri, report)
				}
			}
		} else {
			for _, supi := range sub.UeSupiList {
				if supi == ue.Supi {
					for _, event := range sub.EventSubscription.EventList {
						fmt.Printf("[DEBUG] event.Type=%v, expect=%v\n", event.Type, models.AmfEventType_LOCATION_REPORT)
						if event.Type == "LOCATION_REPORT" || event.Type == models.AmfEventType_LOCATION_REPORT {
							report := makeLocationReport(ue, key.(string))
							fmt.Printf("[DEBUG] Ready to post LOCATION_REPORT for supi=%s to %s\n", ue.Supi, sub.EventSubscription.EventNotifyUri)
							go postToUri(sub.EventSubscription.EventNotifyUri, report)
						}
					}
				}
			}
		}
		return true
	})
}

func makeLocationReport(ue *context.AmfUe, subscriptionId string) map[string]interface{} {
	// 简单版，实际可参考event_exposure.go的newAmfEventReport
	return map[string]interface{}{
		"supi":           ue.Supi,
		"type":           "LOCATION_REPORT",
		"location":       ue.Location,
		"subscriptionId": subscriptionId,
		"timestamp":      time.Now().UTC(),
	}
}

func postToUri(uri string, report map[string]interface{}) {
	fmt.Printf("[DEBUG] postToUri called, uri=%s, report=%v\n", uri, report)
	data, _ := json.Marshal(report)
	resp, err := http.Post(uri, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to post LOCATION_REPORT to %s: %v", uri, err)
		return
	}
	defer resp.Body.Close()
	log.Printf("Posted LOCATION_REPORT to %s, status: %s", uri, resp.Status)
}
func init() {
	go StartLocationReporter()
}
