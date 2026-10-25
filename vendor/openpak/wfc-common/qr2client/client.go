package qr2client

import (
	"context"
	"log"
	"openpak/wfc-common/common"
	"openpak/wfc-common/wfcapi"
)

type GroupInfo = wfcapi.GroupInfo

func call(op string, in wfcapi.QR2Request) (wfcapi.QR2Response, error) {
	var out wfcapi.QR2Response
	err := wfcapi.Call(context.Background(), "WFC_QR2_URL", "/internal/v1/qr2/"+op, in, &out)
	if err != nil {
		log.Printf("QR2 %s failed: %v", op, err)
	}
	return out, err
}
func GetSessionServers() ([]map[string]string, error) {
	r, e := call("servers", wfcapi.QR2Request{})
	return r.Servers, e
}
func GetGroups(games, groups []string, sorted bool) ([]GroupInfo, error) {
	r, e := call("groups", wfcapi.QR2Request{Games: games, Groups: groups, Sorted: sorted})
	return r.Groups, e
}
func GetSearchID(addr uint64, pid uint32, generation string) uint64 {
	r, e := call("search-id", wfcapi.QR2Request{Address: addr, ProfileID: pid, Generation: generation})
	if e != nil {
		return 0
	}
	return r.SearchID
}
func Login(pid uint32, game, name string, fc uint64, fcgame, ip string, exploit, device, restricted bool, generation string, revision int64) error {
	_, e := call("login", wfcapi.QR2Request{Login: &wfcapi.Login{ProfileID: pid, Generation: generation, Revision: revision, GameCode: game, InGameName: name, ConsoleFriendCode: fc, FriendKeyGame: fcgame, GPPublicIP: ip, NeedsExploit: exploit, DeviceAuthenticated: device, Restricted: restricted}})
	return e
}
func Logout(pid uint32, generation string) {
	call("logout", wfcapi.QR2Request{ProfileID: pid, Generation: generation})
}
func SetDeviceAuthenticated(pid uint32, generation string) {
	call("device-authenticated", wfcapi.QR2Request{ProfileID: pid, Generation: generation})
}
func ProcessGPStatusUpdate(pid uint32, addr uint64, status, generation string) {
	call("status", wfcapi.QR2Request{Generation: generation, ProfileID: pid, Address: addr, Status: status})
}
func ProcessGPTellAddr(pid uint32, addr uint64, dest uint32, destAddr uint64, generation string) {
	call("tell-address", wfcapi.QR2Request{Generation: generation, ProfileID: pid, Address: addr, DestProfileID: dest, DestAddress: destAddr})
}
func CheckGPReservationAllowed(addr uint64, pid, dest uint32, join byte, generation string) string {
	r, e := call("reservation-allowed", wfcapi.QR2Request{Generation: generation, ProfileID: pid, Address: addr, DestProfileID: dest, JoinType: join})
	if e != nil {
		return "network_error"
	}
	return r.Allowed
}
func ProcessGPResvOK(version int, reservation common.MatchCommandDataReservation, resv common.MatchCommandDataResvOK, addr uint64, pid uint32, destAddr uint64, dest uint32, generation string) bool {
	r, e := call("reservation-ok", wfcapi.QR2Request{Generation: generation, MatchVersion: version, Reservation: reservation, ResvOK: resv, Address: addr, ProfileID: pid, DestAddress: destAddr, DestProfileID: dest})
	return e == nil && r.Accepted
}
func ProcessUSER(pid uint32, addr uint64, packet []byte, generation string) {
	call("user", wfcapi.QR2Request{Generation: generation, ProfileID: pid, Address: addr, Packet: packet})
}
func ProcessMKWSelectRecord(pid uint32, key, value, generation string) {
	call("selection", wfcapi.QR2Request{Generation: generation, ProfileID: pid, Key: key, Value: value})
}
func SendClientMessage(ip string, search uint64, packet []byte) {
	call("client-message", wfcapi.QR2Request{SenderIP: ip, SearchID: search, Packet: packet})
}
func ProcessNATNEGReport(result byte, ip1, ip2 string) {
	call("natneg-report", wfcapi.QR2Request{Result: result, IP1: ip1, IP2: ip2})
}
