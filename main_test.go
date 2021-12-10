package requests

import (
	"bytes"
	"fmt"
	"testing"
)

func TestCrawler(t *testing.T) {
	EnableLog()

	RegisterAgent("", &Agent{Execute: ExecuteCrawler})

	buf := bytes.NewBuffer([]byte{})
	rw := NewMockResponseWriter(buf)
	rq := NewMockRequest(map[string]string{"nums": "CX102781571AR"})
	Run(rw, rq)

	fmt.Printf("%s\n", buf.String())
}

func ExecuteCrawler(r *Request, trackingNoList []string, lan, postcode, dest, date string) []*TrackingItem {
	var resp *Response
	var err error

	// 1. Get csrf-token
	resp, err = r.Get("https://service.post.ch/ekp-web/ui/list", nil)
	if err != nil {
		panic(err)
	}
	csrfToken := resp.Header("X-Csrf-Token")
	if csrfToken == "" {
		return MakeBatchFatalTrackingItem(trackingNoList, 206, "$无法获取csrf-token$", resp.AsString())
	}

	// 2. Get userId
	resp, err = r.AcceptJSON().Get("https://service.post.ch/ekp-web/api/user", nil)
	if err != nil {
		panic(err)
	}
	userIdRsp := resp.AsJson()
	userId := GetMapString(userIdRsp, "userIdentifier")
	if csrfToken == "" {
		return MakeBatchFatalTrackingItem(trackingNoList, 206, "$无法获取user-id$", resp.AsString())
	}

	result := make([]*TrackingItem, 0)
	// 3. Get hash
	// 多单号查询，不应使用循环，此处是示例。
	for _, trackingNo := range trackingNoList {
		trackingItem := NewTrackingItem(trackingNo)
		result = append(result, trackingItem)

		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("origin", "https://service.post.ch").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Header("referer", "https://service.post.ch/ekp-web/ui/list").
			Header("x-csrf-token", csrfToken).
			PostJson("https://service.post.ch/ekp-web/api/history", map[string]string{"userId": userId}, map[string]string{"searchQuery": trackingNo})
		if err != nil {
			panic(err)
		}
		hashRsp := resp.AsJson()
		hash := GetMapString(hashRsp, "hash")
		if hash == "" {
			trackingItem.Code = 206
			trackingItem.CodeMg = "$无法获取hash$"
			trackingItem.CMess = resp.AsString()
			continue
		}

		// 4. Get identity
		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Get("https://service.post.ch/ekp-web/api/history/not-included/"+hash, map[string]string{"userId": userId})
		if err != nil {
			panic(err)
		}
		identityRsp := resp.AsJsonArray()
		identity := GetMapString(identityRsp[0], "identity")
		if identity == "" {
			trackingItem.Code = 206
			trackingItem.CodeMg = "$无法获取identity$"
			trackingItem.CMess = resp.AsString()
			continue
		}

		// 5. Actually query
		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Get("https://service.post.ch/ekp-web/api/shipment/id/"+identity+"/events/", nil)
		if err != nil {
			panic(err)
		}
		resultRsp := resp.AsJsonArray()

		events := make([]*TrackingEvent, 0)
		sHour := 0
		sMin := 0
		sSec := 0
		for _, resultItem := range resultRsp {
			zip := GetMapString(resultItem, "zip")
			city := GetMapString(resultItem, "city")
			eventCode := GetMapString(resultItem, "eventCode")
			// eventCodeType := GetMapString(resultItem, "eventCodeType")
			// shipmentNumber := GetMapString(resultItem, "shipmentNumber")
			timestamp := ParseDateTime(GetMapString(resultItem, "timestamp"))

			eventValue := ""
			for evk, evv := range eventValueMap {
				if MatchOddityCode(eventCode, evk) {
					eventValue = ReduceWhitespaces(evv)
					break
				}
			}

			FillEmptyClock(&timestamp, &sHour, &sMin, &sSec)

			event := TrackingEvent{
				Place:   ReduceWhitespaces(zip) + " " + ReduceWhitespaces(city),
				Details: eventValue,
				Date:    FormatDateTime(timestamp),
			}
			events = append(events, &event)
		}

		trackingItem.Events = events
	}

	return result
}

var (
	eventValueMap = map[string]string{
		"PARCEL.*.262144":      "Betreibungsurkunde P",
		"PARCEL.*.*.TNT-AA.*":  "Doppelte oder fehlende Sendungsdaten im System",
		"PARCEL.*.*.9116.*":    "Auftrag Nachforschung eingegangen",
		"PARCEL.*.*.TNT-CD.*":  "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-OF.*":  "Sendung an Zustellpartner übergeben",
		"PARCEL.*.*.TNT-WR.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung ",
		"PARCEL.*.*.TNT-MC.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.524288":      "DIRECT",
		"PARCEL.*.*.TNT-CDE.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.41.*":      "Nicht erfolgreiche Zustellung", "LETTER.*.94_LONG": "PRIORITY Plus",
		"PARCEL.*.*.5500.*.MEZ1": "Frist bis {{valueParameter}}",
		"LETTER.*.*.9250.IMPORT": "Importkosten bezahlt",
		"PARCEL.*.*.TNT-WAD.*":   "Fehlerhafter Türcode - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.906.*":       "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.PPIP_LONG":     "\nPostPac International Priority", "LETTER.*.*.929.*": "Fehlleitung",
		"LETTER.*.26.9208.*":   "Der Empfänger hat einen Auftrag ausgelöst: Etagenzustellung\n",
		"PARCEL.*.*.TNT-BS.*":  "Zustellungsdatum auf Wunsch vom Versender oder Empfänger geändert",
		"PARCEL.*.*.854.*":     "Dateneinlieferung durch den Absender",
		"PARCEL.*.*.TNT-DNR.*": "Auf Wunsch des Empfängers Zustellung bei Nachbarn",
		"PARCEL.*.*.TNT-LR.*":  "Mögliche Verzögerung - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.33_LONG":     "SmallPac Economy",
		"PARCEL.*.*.TNT-WAT.*": "Fehlerhafter Orts-/Stadtname - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-NTT.*": "Sendung nicht termingerecht zugestellt - Zustellung baldmöglichst",
		"LETTER.*.106_LONG":    "PRIORITY Plus", "PARCEL.*.PPIE_LONG": "PostPac international Economy",
		"PARCEL.*.5120_LONG":   "SameDay Nachmittag",
		"PARCEL.*.*.TNT-DF.*":  "Verpackung beschädigt - neu verpackt und weitergeleitet",
		"PARCEL.*.*.TNT-LXO.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-CIM.*": "Kundenidentifikation fehlt - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-LEC.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.951.*":     "Ankunft im Logistik Hub",
		"PARCEL.*.*.1302.*":    "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.*.TNT-MOP.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.918.*":     "Ankunft Bestimmungsland",
		"PARCEL.*.*.9202.*":    "Der Empfänger hat einen Auftrag ausgelöst: Zweite Zustellung",
		"PARCEL.*.*.TNT-AR.*":  "Sendung umgeleitet, Verzögerungen möglich - Vorgang in Klärung",
		"PARCEL.*.*.843.*":     "Verzollungsprozess",
		"PARCEL.*.*.TNT-MT.*":  "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.1400.*":    "Ankunft in Spezialzustellung",
		"PARCEL.*.*.820.*":     "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.*.TNT-LCP.*": "Bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.807.*":     "Codierereignis Paketzentrum Teilsortiert",
		"PARCEL.*.*.TNT-MD.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-UP.*":  "Sendung nicht versandfähig - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.928.*":     "Nicht erfolgreiche Zustellung", "LETTER.*.89_LONG": "PRIORITY Plus",
		"PARCEL.*.*.TNT-HM.*": "Weiterleitung verzögert sich - fehlerhafte/fehlende Dokumente",
		"PARCEL.*.*.600.*":    "Datenübermittlung durch Versender", "PARCEL.*.*.853.*": "Verzollungsprozess",
		"PARCEL.*.*.TNT-LS.*": "Verzögerung durch Dritte - Zustellung erfolgt schnellstmöglich",
		"PARCEL.*.*.TNT-DW.*": "Gefahrgut - Sendung wird schnellstmöglich weitergeleitet",
		"PARCEL.*.*.1204.*":   "Sendung wird ausgeschleust",
		"PARCEL.*.*.3800.*":   "Zugestellt im Ablagefach durch", "LETTER.*.82_LONG": "PRIORITY Plus",
		"LETTER.*.*.819.*":     "Verzollungsprozess",
		"LETTER.*.*.36.*.18":   "Briefkasten und/oder Klingel nicht angeschrieben",
		"PARCEL.*.*.TNT-CRI.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-MRC.*": "Verzögerung aufgrund Beschau durch Behörden - Vorgang in Klärung",
		"PARCEL.*.*.9105.*":    "Nachforschung ausgelöst",
		"PARCEL.*.*.TNT-DG.*":  "Gefahrgut - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-LXN.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LC.*":  "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.1303.*":    "Sendung wurde sortiert für die Zustellung",
		"PARCEL.*.*.TNT-LEB.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.952.*":     "Verarbeitung im Logistik Hub",
		"PARCEL.*.*.TNT-VB.*":  "Mögliche Verzögerung in der Zustellung - Vorgang in Klärung",
		"LETTER.*.102_LONG":    "Warensendung Ausland",
		"PARCEL.*.*.TNT-EI.*":  "Verspätete Ausfuhr durch Zollbeschau - Zustellung baldmöglichst",
		"PARCEL.*.4104":        "SameDay GAS",
		"PARCEL.*.*.TNT-MOO.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.9203.*":    "Der Empfänger hat einen Auftrag ausgelöst: Einmalvollmacht",
		"PARCEL.*.*.842.*":     "Verzollungsprozess",
		"PARCEL.*.*.TNT-AS.*":  "Sendung liegt am Umschlagpunkt zur Weiterleitung bereit",
		"PARCEL.*.*.TNT-CV.*":  "Ware im Zoll festgehalten - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-LAL.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.4100":        "SameDay Sperrgut", "LETTER.*.26.40.*.IMP1": "Empfangsbestätigung ausstehend",
		"LETTER.*.26.40.*.IMP2": "Deponierung", "PARCEL.*.3250": "Thermomonitoring",
		"PARCEL.*.*.TNT-RDR.*": "Zustellungsdatum auf Wunsch des Empfängers geändert",
		"LETTER.*.*.91.*.STA5": "ausgelöst",
		"LETTER.*.*.610.*":     "Anmeldung der Sendung durch Verzollung (Dateneinlieferung)",
		"LETTER.*.89":          "PRIORITY Plus", "LETTER.*.88": "Einschreiben Ausland",
		"LETTER.*.*.908.*": "Ankunft an der Verarbeitungs-/Abholstelle", "PARCEL.*.U": "URGENT",
		"LETTER.*.87":         "Import EMS",
		"PARCEL.*.*.TNT-DH.*": "Sendung in der Ausgangsniederlassung eingetroffen",
		"PARCEL.*.*.2101.*":   "Bereit zur Abholung an PickPost-Stelle / My Post 24-Automat",
		"PARCEL.*.*.TNT-UA.*": "Fehlerhafte Adresse - bitte kontaktieren Sie TNT Express",
		"LETTER.*.86":         "Auslandsendung", "LETTER.*.85_LONG": "Einschreiben Ausland",
		"LETTER.*.85": "Einschreiben Ausland", "PARCEL.*.*.TNT-WD.*": "Bitte kontaktieren Sie TNT Express",
		"LETTER.*.84": "Einschreiben Ausland", "LETTER.*.83": "Einschreiben Ausland",
		"PARCEL.*.1073741824": "URGENT", "LETTER.*.82": "PRIORITY Plus", "LETTER.*.81": "PRIORITY Plus",
		"PARCEL.*.*.TNT-OKC.*": "Neue Lieferadresse durch den Empfänger angefragt",
		"LETTER.*.80":          "Warensendung Ausland",
		"PARCEL.*.*.TNT-LT.*":  "Technische Probleme, mögliche Verzögerung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-SYS.*": "Mögliche Verzögerung durch Systemausfall - Vorgang in Klärung",
		"PARCEL.*.*.999.*":     "Unbekanntes Ereignis", "PARCEL.*.*.3800.*.1": "Box bleibt beim Kunden",
		"PARCEL.*.*.9310.*": "Auftrag pick@home", "PARCEL.*.9": "GAS-Economy",
		"PARCEL.*.*.TNT-CRL.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.8192.2100.*": "Zur Abholung gemeldet (Abholungseinladung)",
		"PARCEL.*.*.TNT-RM.*":  "Sendung an Zustellpartner übergeben",
		"PARCEL.*.*.953.*":     "Die Sendung hat den Logistik Hub verlassen",
		"PARCEL.*.*.930.*":     "Behandlung beschädigte Sendung / Adressabklärung",
		"PARCEL.*.*.1304.*":    "Sortierung",
		"PARCEL.*.*.TNT-LD.*":  "Verzögerung durch behördliche Sicherheitsmaßnahmen",
		"PARCEL.*.*.TNT-LEA.*": "Verzögerung aufgrund Beschau durch Behörden - Vorgang in Klärung",
		"LETTER.*.*.34.*.RET0": "Nicht abgeholt", "PARCEL.*.1": "PostPac Economy",
		"LETTER.*.*.34.*.RET1": "Weggezogen, Nachsendefrist abgelaufen", "PARCEL.*.2": "PostPac Priority",
		"PARCEL.*.*.1500.*":      "Behandlung beschädigte Sendung",
		"LETTER.*.*.34.*.RET2":   "Annahme verweigert",
		"LETTER.*.*.34.*.RET3":   "Empfänger konnte unter angegebener Adresse nicht ermittelt werden",
		"PARCEL.*.*.TNT-IP.*":    "Unzureichende Verpackung - Vorgang in Klärung",
		"PARCEL.*.*.845.*":       "Verzollungsprozess",
		"PARCEL.*.*.TNT-VC.*":    "Mündliche Zustellbestätigung des Empfängers",
		"LETTER.*.*.34.*.RET4":   "Gestorben",
		"PARCEL.*.8192.2100.*.2": "Zusatzinformation: Schriftliche Ermächtigung vorhanden",
		"PARCEL.*.5":             "Sperrgut", "LETTER.*.*.34.*.RET5": "Firma erloschen",
		"PARCEL.*.6":           "Sperrgut Priority",
		"LETTER.*.*.34.*.RET6": "Direkt Retour gemäss Anweisung des Absenders",
		"LETTER.*.96":          "PRIORITY Plus", "LETTER.*.*.34.*.RET7": "Beschädigung",
		"PARCEL.*.8192.2100.*.1": "Zusatzinformation: Geschäft bis 09h00 geschlossen",
		"LETTER.*.95":            "PRIORITY Plus", "LETTER.*.*.34.*.RET8": "Postlagernd",
		"LETTER.*.94": "PRIORITY Plus", "LETTER.*.*.34.*.RET9": "Im Militär",
		"LETTER.*.93": "PRIORITY Plus", "PARCEL.*.*.3601.*": "Beginn Rücksendung",
		"LETTER.*.92": "Importsendung", "LETTER.*.91": "Importsendung", "LETTER.*.90": "PRIORITY Plus",
		"LETTER.*.26.54.*.LEG00": "kein Rechtsvorschlag erhoben", "PARCEL.*.0": "PostPac",
		"PARCEL.*.*.TNT-MLX.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.9115.*":    "Kundenreaktion eingegangen",
		"PARCEL.*.*.TNT-OI.*":  "Sendung im Zoll festgehalten - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-MF.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.5400.*":    "Uebergabe an Feldpost", "LETTER.*.*.40.*": "Zugestellt durch ",
		"LETTER.*.*.63.*":      "Avisiert ins Postfach zur Abholung am Schalter ",
		"PARCEL.*.*.920.*":     "Sendung wurde sortiert und weitergeleitet",
		"LETTER.*.*.1300.*":    "Sendung wurde sortiert für die Zustellung",
		"PARCEL.*.*.TNT-WE.*":  "Sendungsabwicklung am Wochenende nicht möglich",
		"PARCEL.*.*.TNT-BF.*":  "Sendung aufgefunden und zum Zielort weitergeleitet",
		"PARCEL.*.*.TNT-HO.*":  "Empfänger nicht erreichbar - Zustellung erfolgt schnellstmöglich",
		"LETTER.*.*.907.*":     "Sendung wurde sortiert für die Zustellung",
		"PARCEL.*.*.TNT-MLH.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.2621440":     "DIRECT + Sonntagszustellung", "LETTER.*.26.54.*.LEG14": "Teilrechtsvorschlag",
		"LETTER.*.26.54.*.LEG13": "Rechtsvorschlag gesamte Forderung", "PARCEL.*.URGENT": "URGENT",
		"PARCEL.*.*.TNT-BSN.*": "Empfänger wünscht Terminvereinbarung vor Zustellversuch",
		"PARCEL.*.5120":        "SameDay Nachmittag",
		"PARCEL.*.*.TNT-CX.*":  "Dokumente und Sendung wurden vertauscht - Vorgang in Klärung",
		"PARCEL.*.*.TNT-MRA.*": "Verzögerung aufgrund Beschau durch Behörden - Vorgang in Klärung",
		"LETTER.*.11_LONG":     "Für den Empfang der Sendung ist eine Unterschrift nötig",
		"PARCEL.*.*.TNT-LXL.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-MTU.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-NH.*":  "Empfänger bei Zustellversuch nicht angetroffen",
		"PARCEL.*.*.931.*":     "nicht erfolgreiche Zustellung: verbotener Artikel",
		"LETTER.*.26.40.*":     "Zugestellt durch ",
		"LETTER.*.*.620.*":     "Anmeldung der Sendung durch ausländischen Versender (Dateneinlieferung)",
		"LETTER.*.*.919.*":     "Verzollungsprozess",
		"LETTER.*.*.91.*.STA6": "abgeschlossen - Sendung zugestellt",
		"PARCEL.*.*.3602.*":    "Rücksendung bearbeitet",
		"PARCEL.*.*.9224.*":    "Der Empfänger hat einen Auftrag ausgelöst: Weiterleitung",
		"LETTER.*.*.91.*.STA7": "abgeschlossen - Sendung nicht zugestellt",
		"LETTER.*.*.91.*.STA8": "erfolglos",
		"PARCEL.*.*.TNT-CAR.*": "Lieferadresse auf Wunsch des Empfängers geändert",
		"PARCEL.*.*.844.*":     "Verzollungsprozess", "LETTER.*.*.91.*.STA9": "abgeschlossen",
		"PARCEL.*.*.9201.*":    "Der Empfänger hat einen Auftrag ausgelöst: Weiterleitung",
		"PARCEL.*.*.TNT-MOC.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.1.*.9501.*":    "Rückzug erfolgreich", "PARCEL.*.6144_LONG": "SameDay Abend",
		"PARCEL.*.*.TNT-BW.*": "Verzögerung durch Wetterverhältnisse - Verladung baldmöglichst",
		"PARCEL.*.*.906.*":    "Sendung wurde sortiert und weitergeleitet", "PARCEL.*.*.929.*": "Fehlleitung",
		"PARCEL.*.*.9703.*": "Wareneingang Tour", "PARCEL.*.*.9112.*": "Nachforschung",
		"PARCEL.*.*.TNT-RDH.*": "Verzögerte Zustellung - begrenzte Öffnungszeiten beim Empfänger",
		"PARCEL.*.131072":      "SwissExpress Sonne", "LETTER.*.*.842.*": "Verzollungsprozess",
		"PARCEL.*.*.TNT-MLO.*":      "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.EMS.1203.IMPORT":  "Codierereignis Paketzentrum Teilsortiert",
		"PARCEL.*.*.TNT-ITC.*":      "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-HP.*":       "Sendung liegt zur Abholung in der Zustellniederlassung bereit",
		"LETTER.*.26.40.*.LEG13":    "Rechtsvorschlag gesamte Forderung",
		"LETTER.*.26.40.*.LEG14":    "Teilrechtsvorschlag",
		"PARCEL.*.*.TNT-UC.*":       "Sendung nicht versandfähig - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.PPIE.1203.IMPORT": "Codierereignis Paketzentrum Teilsortiert",
		"PARCEL.*.*.TNT-WAH.*":      "Fehlerhafte Hausnummer - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.1201.*":         "Sendung wurde sortiert für die Zustellung", "PARCEL.*.4096": "SameDay",
		"PARCEL.*.*.TNT-LXC.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LAA.*": "Verzögerung aufgrund Beschau durch Behörden - Vorgang in Klärung",
		"PARCEL.*.*.TNT-CRF.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-PL.*":  "Gewünschte Abholzeit zu spät für sofortige Weiterleitung",
		"LETTER.*.102":         "Warensendung Ausland", "PARCEL.*.192_LONG": "Rücksendung Vinolog",
		"PARCEL.*.*.3501.*":   "Beginn Nachsendung",
		"PARCEL.*.*.TNT-RO.*": "Sendung zurück an Versender - bitte kontaktieren Sie TNT Express",
		"LETTER.*.101":        "eTracking light", "PARCEL.*.*.TNT-TR.*": "Sendung wurde weitergeleitet",
		"LETTER.*.100": "Warensendung Ausland", "LETTER.*.106": "PRIORITY Plus",
		"LETTER.*.105": "Warensendung Ausland", "LETTER.*.103": "Einschreiben Ausland",
		"LETTER.*.*.600.*":     "Datenübermittlung durch Versender",
		"LETTER.*.*.854.*":     "Dateneinlieferung durch den Absender",
		"LETTER.*.*.10.*":      "Ankunft an der Abhol-/Zustellstelle",
		"PARCEL.*.*.TNT-MCB.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.4001.*":    "Zugestellt durch",
		"PARCEL.*.*.TNT-LEW.*": "Verspätung durch Wetterverhältnisse - Vorgang in Klärung",
		"PARCEL.*.EMS":         "Internationale Sendung",
		"PARCEL.*.*.TNT-IR.*":  "Sendung in der Zustellniederlassung eingetroffen",
		"PARCEL.*.*.TNT-CI.*":  "Sendung in der Ausgangsniederlassung entgegengenommen",
		"LETTER.*.50_LONG":     "Hausservice", "LETTER.*.*.952.*": "Verarbeitung im Logistik Hub",
		"PARCEL.*.6144": "SameDay Abend", "LETTER.*.26.40.*.LEG00": "kein Rechtsvorschlag erhoben",
		"PARCEL.*.*.9221.*":    "Der Empfänger hat einen Auftrag ausgelöst: Wunschtag / Zeitfenster",
		"PARCEL.*.8200":        "Swiss-Express GAS «Mond»",
		"LETTER.1.*.9502.*":    "Rückzug erfolgreich, Sendung vernichtet",
		"PARCEL.*.*.TNT-ATL.*": "Sendung wurde auf Wunsch an vereinbartem Ort hinterlegt",
		"PARCEL.*.*.400.*":     "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.*.TNT-IB.*":  "Dokumente an interne Verzollungsagenten übergeben",
		"PARCEL.*.*.TNT-OK.*":  "Sendung wurde zugestellt",
		"PARCEL.*.*.907.*":     "Sendung wurde sortiert für die Zustellung",
		"LETTER.*.*.46.*":      "Empfangsbestätigung erhalten", "PARCEL.*.*.9702.*": "Wareneingang Tour",
		"LETTER.*.26.39.*.LEG00": "kein Rechtsvorschlag erhoben",
		"PARCEL.*.*.TNT-MH.*":    "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.2100.*":      "Zur Abholung gemeldet (Abholungseinladung)",
		"LETTER.*.*.20.*.CAN1":   "Widerruf", "PARCEL.*.*.819.*": "Verzollungsprozess",
		"PARCEL.*.*.TNT-UD.*":    "Sendung nicht versandfähig - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.909.*":       "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.*.5500.*":      "Aufbewahrungsfrist wurde durch Empfänger verlängert",
		"PARCEL.*.*.TNT-LW.*":    "Verspätung durch Wetterverhältnisse - Vorgang in Klärung",
		"PARCEL.*.*.1200.*":      "Sendung wurde sortiert",
		"PARCEL.*.*.TNT-MNA.*":   "Mögliche Verzögerung durch Naturkatastrophe - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LXB.*":   "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-PM.*":    "Zustellung erfolgt per Post",
		"PARCEL.*.*.3502.*":      "Nachsendung bearbeitet",
		"PARCEL.*.*.3100.*.505":  "Grund: Abholauftrag beim Kunden unbekannt",
		"PARCEL.*.*.3100.*.504":  "Grund: Ware zu gross für Verpackung",
		"PARCEL.*.*.3100.*.503":  "Grund: Ware ungenügend verpackt",
		"PARCEL.*.*.918.*":       "Ankunft Bestimmungsland",
		"PARCEL.*.*.3100.*.502":  "Grund: Sendung nicht frankiert",
		"LETTER.*.*.34.*":        "Uneingeschriebene Rücksendung",
		"LETTER.*.26.39.*.LEG14": "Teilrechtsvorschlag",
		"LETTER.*.26.39.*.LEG13": "Rechtsvorschlag gesamte Forderung",
		"LETTER.*.*.853.*":       "Verzollungsprozess", "PARCEL.*.PPIE": "PostPac international Economy",
		"PARCEL.*.*.TNT-IS.*":  "Sendung in der Umschlagbasis eingetroffen",
		"PARCEL.*.*.TNT-C1D.*": "Empfänger kontaktiert, Kontaktaufnahme mit Versender empfohlen",
		"LETTER.*.105_LONG":    "Warensendung Ausland", "PARCEL.*.*.4000.*": "Zugestellt durch",
		"PARCEL.*.34_LONG": "SmallPac Priority", "PARCEL.*.*.1004.*": "Übergabe an Filiale",
		"PARCEL.*.*.3100.*.501": "Grund: Sendung nicht am vereinbarten Ort",
		"LETTER.*.*.951.*":      "Ankunft im Logistik Hub", "PARCEL.*.PPIP": "\nPostPac International Priority",
		"PARCEL.*.*.3600.*": "Rücksendung", "LETTER.*.95_LONG": "PRIORITY Plus",
		"PARCEL.*.*.TNT-MCA.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.9116.*":    "Auftrag Nachforschung eingegangen",
		"PARCEL.*.*.4100.*":    "Nicht erfolgreiche Zustellung", "LETTER.*.26.54.*": "Uebergabe an Feldpost ",
		"PARCEL.*.*.TNT-IC.*": "Verspätete Ausfuhr durch Zollbeschau - Zustellung baldmöglichst",
		"PARCEL.*.*.TNT-OL.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.844.*":    "Verzollungsprozess", "PARCEL.*.*.501.*": "Abgeholt beim Kunden",
		"LETTER.*.*.20.*":          "Zugestellt am Schalter",
		"LETTER.*.*.1303.*":        "Sendung wurde sortiert für die Zustellung",
		"PARCEL.*.*.TNT-DL.*":      "Mögliche Verzögerung - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-UE.*":      "Sendung nicht versandfähig - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.908.*":         "Ankunft an der Verarbeitungs-/Abholstelle",
		"PARCEL.*.1073741824_LONG": "URGENT", "PARCEL.*.*.TNT-PIB.*": "Bitte kontaktieren Sie TNT Express",
		"LETTER.*.26.20.*": "Zugestellt am Schalter", "PARCEL.*.*.TNT-HB.*": "Sendung in der Verzollung",
		"LETTER.*.100_LONG":    "Warensendung Ausland",
		"PARCEL.*.*.TNT-LXA.*": "Verzögerung aufgrund Beschau durch Behörden - Vorgang in Klärung",
		"PARCEL.*.*.1300.*":    "Sendung wurde sortiert und weitergeleitet",
		"LETTER.*.*.54.*":      "Übergabe an Feldpost",
		"LETTER.*.*.31.*":      "Erfolgloser Abholversuch durch Bote",
		"LETTER.*.*.9202.*":    "Der Empfänger hat einen Auftrag ausgelöst: Zweite Zustellung",
		"PARCEL.*.*.TNT-MOR.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-AH.*":  "Verzögerung beim Transport - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.919.*":     "Verzollungsprozess",
		"LETTER.*.*.931.*":     "nicht erfolgreiche Zustellung: verbotener Artikel",
		"PARCEL.*.*.TNT-LAO.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-ID.*":  "Sendung wird durch Zollbeschau festgehalten",
		"LETTER.*.*.9115.*":    "Kundenreaktion eingegangen", "PARCEL.*.1572864": "DIRECT + Abendzustellung",
		"PARCEL.*.*.9704.*": "Zustellung Kunde", "PARCEL.*.*.2000.*": "Zugestellt am Schalter",
		"PARCEL.*.8192.3800.*": "Zugestellt im Ablagefach durch",
		"PARCEL.*.*.3997.*":    "Widerruf \"Zugestellt via Postfach\"",
		"PARCEL.*.*.500.*":     "Die Sendung wurde beim Versender abgeholt",
		"PARCEL.*.*.6300.*":    "Avisiert ins Postfach zur Abholung am Schalter",
		"LETTER.*.*.820.*":     "Sendung wurde sortiert und weitergeleitet",
		"LETTER.*.*.843.*":     "Verzollungsprozess",
		"LETTER.*.*.21.*":      "Zur Abholung gemeldet (Abholungseinladung)",
		"PARCEL.*.*.3401.*":    "Beginn Rücksendung",
		"PARCEL.*.*.TNT-DM.*":  "Empfänger reklamiert fehlenden Inhalt - Vorgang in Klärung",
		"PARCEL.*.*.TNT-MLL.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.GLS":         "GLS Parcel", "PARCEL.*.*.1202.*": "Sendung wurde sortiert",
		"PARCEL.*.*.909.*":   "Sendung wurde sortiert und weitergeleitet",
		"LETTER.*.90_LONG":   "PRIORITY Plus",
		"PARCEL.1.*.9205.*":  "Der Empfänger hat einen Auftrag ausgelöst: Sendung deponieren",
		"LETTER.*.*.41.*.10": "Kuriersendung erfolglos, Vorladung", "LETTER.*.87_LONG": "Import EMS",
		"PARCEL.*.*.3500.*": "Nachsendung", "LETTER.*.*.855.*": "Ankunft beim Logistik Hub im Aufgabeland",
		"LETTER.*.*.55.*":        "Aufbewahrungsfrist wurde durch Empfänger verlängert ",
		"PARCEL.*.*.TNT-EO.*":    "Gewünschte Leistungen für Zielort nicht erhältlich",
		"LETTER.*.*.9201.*":      "Der Empfänger hat einen Auftrag ausgelöst: Weiterleitung",
		"LETTER.*.*.953.*":       "Die Sendung hat den Logistik Hub verlassen",
		"LETTER.*.*.930.*":       "Behandlung beschädigte Sendung / Adressabklärung",
		"PARCEL.*.*.6300.*.MEZ1": "Frist bis {{valueParameter}}",
		"PARCEL.*.*.TNT-LAN.*":   "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LET.*":   "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.902.*":       "Verzollungsprozess",
		"PARCEL.*.*.925.*":       "nicht erfolgreiche Zustellung: Annahme verweigert",
		"LETTER.*.80_LONG":       "Warensendung Ausland",
		"LETTER.*.*.1.*":         "Zeitpunkt der Aufgabe Ihrer Sendung",
		"PARCEL.*.8192.4000.*":   "Zugestellt durch", "LETTER.*.*.846.*": "Verzollungsprozess",
		"PARCEL.*.*.1702.*":    "Fehlleitung",
		"PARCEL.*.*.TNT-LXW.*": "Verspätung durch Wetterverhältnisse - Vorgang in Klärung",
		"PARCEL.*.*.4200.*":    "Zugestellt durch",
		"PARCEL.*.*.TNT-DN.*":  "Empfänger nicht angetroffen - Sendung beim Nachbarn zugestellt",
		"LETTER.*.*.49.*":      "Rückzustellung Domizil",
		"LETTER.*.*.55.*.MEZ1": "Frist bis {{valueParameter}} ",
		"LETTER.*.*.921.*":     "Ankunft an der Verarbeitungs-/Abholstelle",
		"PARCEL.*.*.913.*":     "Ankunft an der Verarbeitungs-/Abholstelle",
		"PARCEL.*.*.936.*":     "nicht erfolgreiche Zustellung",
		"PARCEL.*.*.4000.*.-1": "Deponiert mit schriftlicher Zustellermächtigung",
		"LETTER.*.*.858.*":     "Verzollung abgeschlossen", "LETTER.*.*.14.*": "Ankunft in Spezialzustellung",
		"PARCEL.*.*.TNT-C1A.*":    "Zustelltermin mit Empfänger vereinbart",
		"PARCEL.*.*.500.*.0":      "Box zurückgenommen",
		"PARCEL.*.*.TNT-HD.*":     "Ware im Zoll festgehalten - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.1001.*":       "Ankunft bei der Zustellstelle",
		"PARCEL.*.*.TNT-CM.*":     "Gesamtsendung entgegengenommen - Zustellung in Einzelsendungen",
		"PARCEL.*.*.805.*":        "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.TNT-RC.*":     "Sendung ist verzollt und wird weitergeleitet",
		"LETTER.*.*.910.*":        "Zur Abholung gemeldet (Abholungseinladung)",
		"LETTER.*.*.9204.*":       "Der Empfänger hat einen Auftrag ausgelöst: Abholfrist verlängert",
		"LETTER.*.*.933.*":        "nicht erfolgreiche Zustellung: Zahlung von Gebühren",
		"PARCEL.*.*.TNT-ML.*":     "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.B2CP_LONG":      "Internationale Sendung",
		"PARCEL.*.*.926.*":        "Aufbewahrungsfrist wurde durch Empfänger verlängert",
		"LETTER.*.*.845.*":        "Verzollungsprozess",
		"PARCEL.*.*.TNT-PNP.*":    "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.2.*":          "Zeitpunkt der Aufgabe Ihrer Sendung",
		"PARCEL.*.*.TNT-C2A.*":    "Empfänger kontaktiert, Zustelltermin vereinbart",
		"LETTER.*.*.40.*.IMP2":    "Deponierung",
		"LETTER.*.*.3607.*.RET17": "ungültiges/falsches Ausweisdokument",
		"LETTER.*.*.3607.*.RET16": "Siehe Hinweis auf der Sendung",
		"LETTER.*.*.3607.*.RET15": "Kuriersendung erfolglos - Vorladung",
		"LETTER.*.*.3607.*.RET14": "Auftrag Post zurückbehalten",
		"LETTER.*.*.3607.*.RET13": "Rücksendung durch Empfänger",
		"LETTER.*.*.3607.*.RET12": "Briefkasten/Postfach wird nicht mehr geleert",
		"LETTER.*.*.3607.*.RET11": "Ausserhalb Betreibungskreis",
		"LETTER.*.*.40.*.IMP1":    "Empfangsbestätigung ausstehend", "LETTER.*.*.3607.*.RET10": "Im Ausland",
		"LETTER.*.*.920.*":     "Sendung wurde sortiert und weitergeleitet",
		"PARCEL.*.*.TNT-WAM.*": "Empfänger verzogen - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-FR.*":  "Sendung umgeleitet, mögliche Verzögerung - Vorgang in Klärung",
		"PARCEL.*.*.9299.*":    "Der Empfänger hat einen Auftrag annuliert",
		"PARCEL.*.*.TNT-MTO.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.937.*":     "nicht erfolgreiche Zustellung", "PARCEL.*.URGENT_LONG": "URGENT",
		"LETTER.*.*.91.*": "Nachforschung", "PARCEL.*.8192.2100.*.MEZ1": "Frist bis {{valueParameter}}",
		"PARCEL.*.*.TNT-FB.*":  "Sendung an Fluggesellschaft weitergeleitet",
		"PARCEL.*.*.1000.*":    "Ankunft an der Abhol-/Zustellstelle",
		"PARCEL.*.*.804.*":     "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.TNT-LAT.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.15.*":      "Behandlung beschädigte Sendung",
		"LETTER.*.*.38.*":      "Zugestellt im Ablagefach durch",
		"PARCEL.*.*.TNT-LUR.*": "Hinterlegung der Sendung an mit dem Empfänger vereinbartem Ort",
		"PARCEL.*.*.TNT-PA.*":  "Zustellung erfolgt per Post - Zustellbeleg auf Anfrage",
		"LETTER.*.92_LONG":     "Importsendung", "PARCEL.*.16384": "PostPac Promo", "PARCEL.*.16386": "P16386",
		"PARCEL.*.*.TNT-CRA.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-C1P.*": "Empfänger wegen fehlerhafter Telefonnummer nicht erreicht",
		"LETTER.*.*.9203.*":    "Der Empfänger hat einen Auftrag ausgelöst: Einmalvollmacht",
		"LETTER.*.*.932.*":     "nicht erfolgreiche Zustellung: Eingeschränkte Artikel",
		"LETTER.*.10_LONG":     "Für den Empfang der Sendung ist eine Unterschrift nötig.",
		"PARCEL.*.*.904.*":     "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.927.*":     "nicht erfolgreiche Zustellung: Empfänger im Ausstand",
		"LETTER.*.*.848.*":     "Verzollungsprozess", "PARCEL.*.*.9701.*": "Abholung Kunde",
		"LETTER.*.*.47.*":       "Rückzustellung Schalter",
		"PARCEL.*.*.TNT-IG.*":   "Sendung wird zwecks Beschau durch Behörden festgehalten",
		"LETTER.*.*.802.*":      "Verzollungsprozess",
		"PARCEL.*.*.TNT-WL.*":   "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.34.*.RET10": "Im Ausland",
		"PARCEL.*.*.TNT-C2D.*":  "Empfänger kontaktiert, Kontaktaufnahme mit Versender empfohlen",
		"PARCEL.*.*.TNT-NPD.*":  "Empfänger wünscht Avis vor Zustellversuch",
		"PARCEL.*.*.TNT-BM.*":   "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-CRT.*":  "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.818.*":      "Ankunft Bestimmungsland",
		"LETTER.*.*.923.*":      "nicht erfolgreiche Zustellung: Empfänger unbekannt",
		"PARCEL.*.*.TNT-DP.*":   "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-MTL.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-MLA.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LAC.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-CRD.*":  "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-RU.*":   "Zustellhindernis beseitigt - Zustellung erfolgt baldmöglichst",
		"PARCEL.*.*.TNT-WET.*":  "Mögliche Verzögerung - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.200.*":      "Zeitpunkt der Aufgabe Ihrer Sendung", "PARCEL.*.*.938.*": "Zugestellt durch",
		"PARCEL.*.*.915.*": "Abgang Grenzstelle Aufgabeland", "LETTER.*.*.3607.*": "Rücksendung ",
		"PARCEL.*.*.TNT-WAN.*":  "Fehlerhafter Empfängername - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-PR.*":   "Einzelne Packstücke vom Absender erhalten - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LEI.*":  "Verzögerung durch lokale Streiks / Unruhen",
		"LETTER.*.*.12.*":       "Sendung wurde sortiert für die Zustellung",
		"LETTER.*.*.34.*.RET17": "ungültiges/falsches Ausweisdokument",
		"LETTER.*.*.34.*.RET16": "Siehe Hinweis auf der Sendung", "LETTER.*.*.35.*": "Nachsendung",
		"LETTER.*.*.34.*.RET15": "Kuriersendung erfolglos - Vorladung",
		"LETTER.*.*.36.*.RET0":  "Nicht abgeholt", "LETTER.*.*.34.*.RET14": "Auftrag Post zurückbehalten",
		"LETTER.*.*.36.*.RET1":  "Weggezogen, Nachsendefrist abgelaufen",
		"LETTER.*.*.34.*.RET13": "Rücksendung durch Empfänger",
		"LETTER.*.*.34.*.RET12": "Briefkasten/Postfach wird nicht mehr geleert",
		"LETTER.*.*.34.*.RET11": "Ausserhalb Betreibungskreis", "LETTER.*.*.36.*.RET4": "Gestorben",
		"LETTER.*.*.36.*.RET5": "Firma erloschen", "PARCEL.*.*.1003.*": "Verlad in Zustellfahrzeug",
		"PARCEL.*.*.TNT-C1C.*": "Abholtermin in der Niederlassung mit Empfänger vereinbart",
		"LETTER.*.*.36.*.RET2": "Annahme verweigert",
		"LETTER.*.*.36.*.RET3": "Empfänger konnte unter angegebener Adresse nicht ermittelt werden",
		"PARCEL.*.*.807.*":     "Codierereignis Paketzentrum Teilsortiert",
		"LETTER.*.*.36.*.RET8": "Postlagernd",
		"LETTER.*.*.34.*.18":   "Briefkasten und/oder Klingel nicht angeschrieben",
		"LETTER.*.*.36.*.RET9": "Im Militär",
		"LETTER.*.*.36.*.RET6": "Direkt Retour gemäss Anweisung des Absenders",
		"LETTER.*.*.36.*.RET7": "Beschädigung", "LETTER.*.*.912.*": "Zeitpunkt der Aufgabe Ihrer Sendung",
		"LETTER.*.*.935.*":     "nicht erfolgreiche Zustellung",
		"PARCEL.*.*.TNT-CO.*":  "Empfänger nicht angetroffen - neuer Zustellversuch erfolgt",
		"PARCEL.*.*.4000.*.0":  "Box zurückgenommen",
		"PARCEL.*.*.TNT-MOD.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.999999999":   "DIRECTAZS", "PARCEL.*.*.928.*": "Nicht erfolgreiche Zustellung",
		"LETTER.*.*.847.*":    "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.TNT-SG.*": "Sendung wird zwecks Beschau durch Behörden festgehalten",
		"LETTER.*.*.801.*":    "Verzollungsprozess", "PARCEL.*.*.4000.*.1": "Box bleibt beim Kunden",
		"PARCEL.*.*.TNT-LXT.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.192":         "Rücksendung Vinolog",
		"PARCEL.*.*.TNT-C2C.*": "Abholtermin in der Niederlassung mit Empfänger vereinbart",
		"LETTER.*.*.48.*":      "Rückzustellung Postfach",
		"PARCEL.*.*.TNT-DQ.*":  "Verspätete Zustellung durch Wartezeiten beim Empfänger",
		"PARCEL.*.*.TNT-HW.*":  "Sendung in der Niederlassung - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.922.*":     "nicht erfolgreiche Zustellung: Adresse ungültig",
		"PARCEL.*.*.TNT-MRI.*": "Verzögerung durch lokale Streiks / Unruhen",
		"PARCEL.*.*.TNT-LAB.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.8196":        "Swiss-Express Sperrgut «Mond»", "LETTER.*.84_LONG": "Einschreiben Ausland",
		"PARCEL.*.*.TNT-CRC.*":   "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-LM.*":    "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.3600.*.0":    "Grund: Nicht abgeholt",
		"PARCEL.*.8192.3800.*.1": "Zusatzinformation: Geschäft bis 09h00 geschlossen",
		"LETTER.*.*.859.*":       "Dateneinlieferung durch den Absender",
		"PARCEL.*.*.3600.*.1":    "Grund: Weggezogen, Nachsendefrist abgelaufen",
		"PARCEL.*.8192":          "Swiss-Express «Mond»", "PARCEL.*.GLS_LONG": "GLS Parcel",
		"LETTER.*.*.9500.*": "Auftrag Rückzug eingegangen",
		"LETTER.*.*.13.*":   "Sendung wurde sortiert und weitergeleitet",
		"LETTER.*.*.813.*":  "Ankunft an der Verarbeitungs-/Abholstelle", "LETTER.*.*.36.*": "Rücksendung ",
		"PARCEL.*.8192.3800.*.2":    "Zusatzinformation: Schriftliche Ermächtigung vorhanden",
		"PARCEL.*.*.TNT-IAO.*":      "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-HG.*":       "Sendung an Zustellpartner übergeben",
		"PARCEL.*.PPIP.1203.IMPORT": "Codierereignis Paketzentrum Teilsortiert",
		"PARCEL.*.*.TNT-MOU.*":      "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.3508.*":         "Nachsendung Ausland", "PARCEL.*.*.TNT-PC.*": "Sendung wird verzollt",
		"PARCEL.*.*.2000.*.CANPDS": "Widerruf", "PARCEL.*.*.3600.*.4": "Grund: Gestorben",
		"PARCEL.*.*.3600.*.5":      "Grund: Firma erloschen",
		"PARCEL.*.*.3600.*.2":      "Grund: Annahme verweigert",
		"PARCEL.*.*.3600.*.3":      "Grund: Empfänger konnte unter angegebener Adresse nicht ermittelt werden",
		"PARCEL.*.*.TNT-CP.*":      "Einzelne Packstücke vom Absender erhalten - Vorgang in Klärung",
		"LETTER.*.*.934.*":         "nicht erfolgreiche Zustellung: Sendung unzustellbar",
		"PARCEL.*.*.3600.*.6":      "Grund: Retour gemäss Anweisung Absender/Empfänger",
		"LETTER.*.*.911.*":         "Importprozess im Bestimmungsland abgebrochen",
		"PARCEL.*.*.4000.*.CANPDS": "Widerruf", "PARCEL.*.*.3600.*.7": "Grund: Beschädigung",
		"LETTER.*.*.804.*":         "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.TNT-OR.*":      "Sendung im Zoll festgehalten - Dokumente werden erwartet",
		"PARCEL.*.*.TNT-BSA.*":     "Bestätigung des benötigten Zustelltermins vom Empfänger erwartet",
		"PARCEL.*.*.3900.*.CANPDS": "Widerruf",
		"PARCEL.*.*.921.*":         "Ankunft an der Verarbeitungs-/Abholstelle",
		"LETTER.*.93_LONG":         "PRIORITY Plus",
		"LETTER.*.*.925.*":         "nicht erfolgreiche Zustellung: Annahme verweigert",
		"PARCEL.*.*.2100.*.MEZ1":   "Frist bis {{valueParameter}}",
		"PARCEL.*.*.858.*":         "Verzollung abgeschlossen",
		"PARCEL.*.*.TNT-MTB.*":     "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.100.*":         "Zeitpunkt der Aufgabe Ihrer Sendung", "LETTER.*.40": "Dispomail",
		"PARCEL.*.*.TNT-HX.*": "Verzögerung durch Verzollung - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.5102.*":   "Zurückbehalten gemäss Auftrag des Empfängers ",
		"PARCEL.*.*.TNT-BO.*": "Sendung wartet auf Zollfreigabe", "LETTER.*.*.902.*": "Verzollungsprozess",
		"PARCEL.*.*.TNT-C2N.*": "Empfänger nicht erreicht - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.3100.*.2":  "Grund: Kunde lehnt Übergabe ab",
		"PARCEL.*.*.3100.*.1":  "Grund: Kunde nicht anwesend",
		"PARCEL.*.*.3100.*.6":  "Grund: Sendung nicht frankiert",
		"PARCEL.*.*.3100.*.5":  "Grund: Sendung nicht am vereinbarten Ort",
		"LETTER.*.103_LONG":    "Einschreiben Ausland",
		"PARCEL.*.*.TNT-WAP.*": "Postfachadresse - bitte kontaktieren Sie TNT Express",
		"PARCEL.1.*.9220.*":    "Der Empfänger hat einen Auftrag ausgelöst: Zustellgenehmigung",
		"PARCEL.*.*.3100.*.4":  "Grund: Adresse unbekannt", "PARCEL.*.*.9108.*": "Nachforschung erfolglos",
		"PARCEL.*.*.3100.*.3":   "Grund: Sendungsnummer stimmt nicht mit Auftragsnummer überein",
		"PARCEL.*.*.TNT-CRN.*":  "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-HH.*":   "Sendung erfordert spezielle Bearbeitung - Vorgang in Klärung",
		"PARCEL.*.*.932.*":      "nicht erfolgreiche Zustellung: Eingeschränkte Artikel",
		"PARCEL.*.*.3900.*":     "Zugestellt via Postfach",
		"LETTER.*.*.94.*":       "Sendung postlagernd in Poststelle und bereit zur Abholung",
		"PARCEL.*.*.TNT-DB.*":   "Sendungsdokumente an Verzollungsagenten/Empfänger übergeben",
		"PARCEL.*.*.1502.*":     "Adressabklärung",
		"LETTER.*.*.36.*.RET17": "ungültiges/falsches Ausweisdokument",
		"PARCEL.*.*.801.*":      "Verzollungsprozess", "LETTER.*.*.937.*": "nicht erfolgreiche Zustellung",
		"LETTER.*.*.5.*":   "Die Sendung wurde beim Versender abgeholt",
		"PARCEL.*.*.847.*": "Verzollungsprozess abgeschlossen", "LETTER.*.*.18.*": "Verspätete Ankunft",
		"PARCEL.*.*.TNT-C1M.*": "Telefonische Nachricht beim Empfänger hinterlassen",
		"PARCEL.*.34":          "SmallPac Priority", "LETTER.*.50": "Hausservice",
		"PARCEL.*.*.TNT-LEO.*":  "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-LAI.*":  "Verzögerung durch lokale Streiks / Unruhen",
		"PARCEL.*.*.3100.*.9":   "Grund: Abholauftrag beim Kunden unbekannt",
		"LETTER.*.*.36.*.RET10": "Im Ausland",
		"PARCEL.*.*.TNT-POU.*":  "Mögliche Verzögerung durch Stromausfall - Vorgang in Klärung",
		"PARCEL.*.*.TNT-OS.*":   "Sendung wurde von der Abgangsniederlassung weitergeleitet",
		"PARCEL.*.*.3100.*.8":   "Grund: Ware zu gross für Verpackung",
		"LETTER.*.*.36.*.RET11": "Ausserhalb Betreibungskreis",
		"PARCEL.*.*.3100.*.7":   "Grund: Ware ungenügend verpackt",
		"LETTER.*.*.36.*.RET12": "Briefkasten/Postfach wird nicht mehr geleert",
		"PARCEL.*.33":           "SmallPac Economy", "LETTER.*.*.36.*.RET13": "Rücksendung durch Empfänger",
		"PARCEL.*.*.TNT-AN.*":    "Zielort derzeit nicht erreichbar - Vorgang in Klärung",
		"LETTER.*.*.36.*.RET14":  "Auftrag Post zurückbehalten",
		"LETTER.*.*.36.*.RET15":  "Kuriersendung erfolglos - Vorladung",
		"LETTER.*.*.36.*.RET16":  "Siehe Hinweis auf der Sendung",
		"PARCEL.*.*.922.*":       "nicht erfolgreiche Zustellung: Adresse ungültig",
		"LETTER.*.26.20.*.LEG00": "kein Rechtsvorschlag erhoben", "LETTER.*.*.849.*": "Verzollungsprozess",
		"LETTER.*.35_LONG":    "Brief, Import",
		"PARCEL.*.*.TNT-MP.*": "Sendung wurde nicht abgeholt - bitte kontaktieren Sie TNT Express",
		"LETTER.1.*.9210.*":   "Der Empfänger hat einen Auftrag ausgelöst: Zustellgenehmigung",
		"LETTER.*.88_LONG":    "Einschreiben Ausland", "LETTER.*.83_LONG": "Einschreiben Ausland",
		"PARCEL.*.*.TNT-OC.*":    "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"LETTER.*.*.803.*":       "Verzollungsprozess",
		"PARCEL.*.8192.4000.*.1": "Zusatzinformation: Geschäft bis 09h00 geschlossen",
		"PARCEL.*.8192.4000.*.2": "Zusatzinformation: Schriftliche Ermächtigung vorhanden",
		"PARCEL.*.*.TNT-CA.*":    "Lieferadresse auf Wunsch vom Versender oder Empfänger geändert",
		"PARCEL.*.*.TNT-ED.*":    "Sendungsdaten fehlerhaft oder verspätet vom Kunden übermittelt",
		"PARCEL.*.*.TNT-MTC.*":   "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-C2M.*":   "Telefonische Nachricht beim Empfänger hinterlassen",
		"PARCEL.*.*.TNT-PU.*":    "Sendung wurde beim Versender abgeholt",
		"LETTER.*.*.924.*":       "nicht erfolgreiche Zustellung: Empfänger abwesend",
		"PARCEL.*.*.TNT-DS.*":    "Verzögerung durch Zollstreik - Zustellung erfolgt baldmöglichst",
		"LETTER.*.*.901.*":       "Verzollungsprozess", "PARCEL.*.U_LONG": "URGENT",
		"PARCEL.*.*.933.*":       "nicht erfolgreiche Zustellung: Zahlung von Gebühren",
		"LETTER.*.26.20.*.LEG14": "Teilrechtsvorschlag",
		"LETTER.*.26.20.*.LEG13": "Rechtsvorschlag gesamte Forderung",
		"PARCEL.*.*.1900.*":      "Behandlung beschädigte Verpackung",
		"PARCEL.*.*.TNT-RH.*":    "Bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-HI.*":    "Sendung im Zoll festgehalten - Anweisungen vom Empfänger nötig",
		"LETTER.*.26.39.*.CAN1":  "Widerruf",
		"PARCEL.*.*.TNT-DC.*":    "Zustellhindernis beseitigt - Zustellung erfolgt schnellstmöglich",
		"PARCEL.*.*.910.*":       "Zur Abholung gemeldet (Abholungseinladung)",
		"LETTER.*.*.19.*":        "Behandlung beschädigte Verpackung",
		"LETTER.*.*.936.*":       "nicht erfolgreiche Zustellung", "PARCEL.*.*.846.*": "Verzollungsprozess",
		"LETTER.*.26.39.*": "Zugestellt via Postfach", "PARCEL.*.13": "GAS-Sperrgut",
		"PARCEL.*.*.TNT-LEN.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.10":          "GAS-Priority", "LETTER.*.*.913.*": "Ankunft an der Verarbeitungs-/Abholstelle",
		"PARCEL.*.17":          "GAS-Recycling",
		"PARCEL.*.*.TNT-CR.*":  "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.14":          "GAS-Sperrgut-Prio",
		"PARCEL.*.*.923.*":     "nicht erfolgreiche Zustellung: Empfänger unbekannt",
		"PARCEL.*.*.TNT-BSC.*": "Zustelltermin auf Wunsch von Absender / Empfänger verändert",
		"PARCEL.*.*.TNT-OD.*":  "Sendung wird zugestellt", "LETTER.*.86_LONG": "Auslandsendung",
		"LETTER.*.*.904.*":  "Verzollungsprozess abgeschlossen",
		"LETTER.*.*.927.*":  "nicht erfolgreiche Zustellung: Empfänger im Ausstand",
		"PARCEL.*.*.1700.*": "Fehlleitung", "PARCEL.*.65541": "Militärsendung Sperrgut",
		"PARCEL.*.*.TNT-C2P.*":   "Empfänger wegen fehlerhafter Telefonnummer nicht erreicht",
		"PARCEL.*.*.TNT-LP.*":    "Einzelne Packstücke zugestellt - Vorgang in Klärung",
		"PARCEL.*.65":            "Rücksendung",
		"PARCEL.*.*.9400.*":      "Sendung postlagernd in Poststelle und bereit zur Abholung",
		"PARCEL.*.*.TNT-HPR.*":   "Abholung in der Niederlassung mit Empfänger vereinbart",
		"PARCEL.*.69":            "Rücksendung Sperrgut",
		"PARCEL.*.*.5100.*":      "Zurückbehalten gemäss Auftrag des Empfängers ",
		"PARCEL.*.*.911.*":       "Importprozess im Bestimmungsland abgebrochen",
		"PARCEL.*.*.934.*":       "nicht erfolgreiche Zustellung: Sendung unzustellbar",
		"PARCEL.*.*.815.EXPORT":  "Abgang Grenzstelle Aufgabeland",
		"LETTER.*.*.3607.*.RET0": "Nicht abgeholt", "LETTER.*.*.818.*": "Ankunft Bestimmungsland",
		"LETTER.*.*.3607.*.RET2": "Annahme verweigert",
		"PARCEL.*.*.TNT-DD.*":    "Sendung beschädigt ausgeliefert - bitte kontaktieren Sie TNT",
		"PARCEL.*.*.9106.*":      "Nachforschung abgeschlossen - Sendung zugestellt",
		"LETTER.*.*.3607.*.RET1": "Weggezogen, Nachsendefrist abgelaufen",
		"LETTER.*.*.9503.*":      "Rückzug nicht erfolgreich", "LETTER.*.*.3607.*.RET4": "Gestorben",
		"LETTER.*.*.3607.*.RET3": "Empfänger konnte unter angegebener Adresse nicht ermittelt werden",
		"LETTER.*.*.3607.*.RET6": "Direkt Retour gemäss Anweisung des Absenders",
		"PARCEL.*.65538":         "Militärsendung",
		"PARCEL.*.*.TNT-LXI.*":   "Verzögerung durch lokale Streiks / Unruhen",
		"LETTER.*.*.3607.*.RET5": "Firma erloschen", "LETTER.*.*.3607.*.RET8": "Postlagernd",
		"PARCEL.*.*.TNT-CRP.*":   "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.3607.*.RET7": "Beschädigung", "PARCEL.*.65537": "Militärsendung",
		"PARCEL.*.*.849.*": "Verzollungsprozess", "PARCEL.*.*.803.*": "Verzollungsprozess",
		"PARCEL.*.*.9250.IMPORT": "Importkosten bezahlt", "LETTER.*.*.3607.*.RET9": "Im Militär",
		"LETTER.*.12": "Einschreiben Prepaid", "LETTER.*.11": "Einschreiben Inland",
		"LETTER.*.10": "Einschreiben Inland", "LETTER.*.*.4020.*": "Abgang ins Ausland",
		"PARCEL.*.*.3100.*":    "Erfolgloser Abholversuch durch Bote",
		"PARCEL.*.*.TNT-LAW.*": "Verspätung durch Wetterverhältnisse - Vorgang in Klärung",
		"LETTER.*.*.39.*":      "Zugestellt via Postfach ",
		"PARCEL.*.*.TNT-MR.*":  "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-CS.*":  "Verzögerung durch Zollstreik - Zustellung erfolgt baldmöglichst",
		"PARCEL.*.*.9204.*":    "Der Empfänger hat einen Auftrag ausgelöst: Abholfrist verlängert",
		"PARCEL.*.128":         "VinoLog",
		"PARCEL.*.*.TNT-AP.*":  "Fällige Zahlungen offen (Zölle, Steuern und/oder Fracht)",
		"PARCEL.*.32769":       "Blindensendung", "PARCEL.*.*.901.*": "Verzollungsprozess",
		"LETTER.*.*.805.*":  "Verzollungsprozess abgeschlossen",
		"PARCEL.*.*.924.*":  "nicht erfolgreiche Zustellung: Empfänger abwesend",
		"PARCEL.*.*.4600.*": "Empfangsbestätigung erhalten",
		"LETTER.*.*.41.*.1": "Auftrag \"Post zurückbehalten\"", "LETTER.*.29": "Brief mit ID-Check",
		"LETTER.*.*.41.*.2": "Annahmeverweigerung", "LETTER.*.28": "Brief mit Vertragsunterzeichnung",
		"LETTER.*.91_LONG": "Importsendung", "LETTER.*.*.41.*.3": "Empfänger unbekannt",
		"LETTER.*.27": "Brief mit Vertragsunterzeichnung", "LETTER.*.*.41.*.4": "Nachsendeauftrag",
		"LETTER.*.26": "Betreibungsurkunde", "LETTER.*.*.41.*.5": "Sendung beschädigt",
		"LETTER.*.96_LONG": "PRIORITY Plus", "LETTER.*.25": "Uneingeschriebene Nachnahme",
		"PARCEL.*.*.TNT-WAC.*": "Fehlerhafte Kontaktdaten - bitte kontaktieren Sie TNT Express",
		"LETTER.*.*.41.*.6":    "Anweisung auf Sendung", "LETTER.*.24": "Uneingeschriebene Nachnahme",
		"LETTER.*.*.41.*.7": "Briefkasten wird nicht geleert",
		"PARCEL.*.*.813.*":  "Ankunft an der Verarbeitungs-/Abholstelle",
		"LETTER.*.*.926.*":  "Aufbewahrungsfrist wurde durch Empfänger verlängert",
		"LETTER.*.*.41.*.8": "anderer Grund", "LETTER.*.22": "Gerichtsurkunde",
		"LETTER.*.21": "A-Post Plus", "LETTER.*.20": "A-Post Plus",
		"PARCEL.*.*.859.*":    "Dateneinlieferung durch den Absender",
		"PARCEL.*.*.TNT-NT.*": "Sendung nicht termingerecht zugestellt - Zustellung baldmöglichst",
		"PARCEL.*.*.TNT-WA.*": "Fehlerhafte Adresse - bitte kontaktieren Sie TNT Express",
		"PARCEL.*.*.TNT-DU.*": "Sendung beschädigt - bitte kontaktieren Sie TNT Express",
		"LETTER.*.101_LONG":   "eTracking light", "LETTER.*.*.40.*.CAN1": "Widerruf",
		"PARCEL.*.*.912.*":     "Zeitpunkt der Aufgabe Ihrer Sendung",
		"PARCEL.*.*.TNT-WAS.*": "Fehlerhafter Strassenname - bitte kontaktieren Sie TNT Express",
		"LETTER.*.81_LONG":     "PRIORITY Plus", "PARCEL.*.*.935.*": "nicht erfolgreiche Zustellung",
		"PARCEL.*.*.TNT-LEL.*": "Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.B2CP":        "Internationale Sendung",
		"PARCEL.*.*.9107.*":    "Nachforschung abgeschlossen - Sendung nicht zugestellt",
		"PARCEL.*.*.TNT-LA.*":  "Verzögerung durch Stau/Unfall des Transportfahrzeugs",
		"PARCEL.*.*.TNT-CRO.*": "Empfänger verweigert Annahme - bitte kontaktieren Sie TNT Express",
		"LETTER.*.36":          "Import ohne Nachnahme", "PARCEL.*.*.3901.*": "Zugestellt Postfach",
		"LETTER.*.*.938.*": "Zugestellt durch", "LETTER.*.35": "Brief, Import",
		"PARCEL.*.*.802.*": "Verzollungsprozess", "LETTER.*.*.915.*": "Abgang Grenzstelle Aufgabeland",
		"LETTER.*.*.17.*": "Fehlleitung", "PARCEL.*.*.848.*": "Verzollungsprozess",
		"PARCEL.*.*.1800.*": "Verspätete Ankunft", "PARCEL.*.32770": "Blindensendung",
		"PARCEL.*.*.TNT-MOI.*": "Mögliche Verzögerung in der Weiterleitung - Vorgang in Klärung",
		"PARCEL.*.*.TNT-C1N.*": "Empfänger telefonisch nicht erreicht",
	}
)
