// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package nextcloud

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Response contains data for the Nextcloud mock server to respond
// and to switch to a new server state
type Response struct {
	code           int
	body           string
	newServerState string
}

const serverStateError = "ERROR"
const serverStateEmpty = "EMPTY"
const serverStateHome = "HOME"
const serverStateSubdir = "SUBDIR"
const serverStateNewdir = "NEWDIR"
const serverStateSubdirNewdir = "SUBDIR-NEWDIR"
const serverStateFileRestored = "FILE-RESTORED"
const serverStateGrantAdded = "GRANT-ADDED"
const serverStateGrantUpdated = "GRANT-UPDATED"
const serverStateRecycle = "RECYCLE"
const serverStateReference = "REFERENCE"
const serverStateMetadata = "METADATA"

var serverState = serverStateEmpty

var responses = map[string]Response{
	`POST /apps/sciencemesh/~einstein/api/storage/AddGrant {"path":"/subdir"}`: {200, ``, serverStateGrantAdded},

	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/subdir"} EMPTY`:  {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/subdir"} HOME`:   {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/subdir"} NEWDIR`: {200, ``, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/newdir"} EMPTY`:  {200, ``, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/newdir"} HOME`:   {200, ``, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/storage/CreateDir {"path":"/newdir"} SUBDIR`: {200, ``, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/storage/CreateHome `:   {200, ``, serverStateHome},
	`POST /apps/sciencemesh/~einstein/api/storage/CreateHome {}`: {200, ``, serverStateHome},

	`POST /apps/sciencemesh/~einstein/api/storage/CreateReference {"path":"/Shares/reference"}`: {200, `[]`, serverStateReference},

	`POST /apps/sciencemesh/~einstein/api/storage/Delete {"path":"/subdir"}`: {200, ``, serverStateRecycle},

	`POST /apps/sciencemesh/~einstein/api/storage/EmptyRecycle `: {200, ``, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/"},"mdKeys":null} EMPTY`: {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/"},"mdKeys":null} HOME`:  {200, `{ "size": 1, "path":"/", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateHome},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/newdir"},"mdKeys":null} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/newdir"},"mdKeys":null} HOME`:          {404, ``, serverStateHome},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/newdir"},"mdKeys":null} SUBDIR`:        {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/newdir"},"mdKeys":null} NEWDIR`:        {200, `{ "size": 1, "path":"/newdir", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/newdir"},"mdKeys":null} SUBDIR-NEWDIR`: {200, `{ "size": 1, "path":"/newdir", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/new_subdir"},"mdKeys":null}`: {200, `{ "size": 1 }`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} HOME`:          {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} NEWDIR`:        {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} RECYCLE`:       {404, ``, serverStateRecycle},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} SUBDIR`:        {200, `{ "size": 1, "path":"/subdir", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} SUBDIR-NEWDIR`: {200, `{ "size": 1, "path":"/subdirh", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdir"},"mdKeys":null} METADATA`:      {200, `{ "size": 1,, "path":"/subdir", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateMetadata},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdirRestored"},"mdKeys":null} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdirRestored"},"mdKeys":null} RECYCLE`:       {404, ``, serverStateRecycle},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdirRestored"},"mdKeys":null} SUBDIR`:        {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/subdirRestored"},"mdKeys":null} FILE-RESTORED`: {200, `{ "size": 1, "path":"/subdirRestored", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/versionedFile"},"mdKeys":null} EMPTY`:         {200, `{ "size": 2, "path":"/versionedFile", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/GetMD {"ref":{"path":"/versionedFile"},"mdKeys":null} FILE-RESTORED`: {200, `{ "size": 1, "path":"/versionedFile", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/storage/GetPathByID {"storage_id":"00000000-0000-0000-0000-000000000000","opaque_id":"fileid-%2Fsubdir"}`: {200, "/subdir", serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/InitiateUpload {"path":"/file"}`: {200, `{"simple": "yes","tus": "yes"}`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/ListFolder {"ref":{"path":"/"},"mdKeys":null}`: {200, `[{ "size": 1, "path":"/subdir", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }]`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/ListFolder {"ref":{"path":"/Shares"},"mdKeys":null} EMPTY`:     {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/ListFolder {"ref":{"path":"/Shares"},"mdKeys":null} SUBDIR`:    {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/ListFolder {"ref":{"path":"/Shares"},"mdKeys":null} REFERENCE`: {200, `[{ "size": 1, "path":"/Shares/reference", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }]`, serverStateReference},

	`POST /apps/sciencemesh/~einstein/api/storage/ListGrants {"ref":{"path":"/subdir"},"mdKeys":null} SUBDIR`:        {200, `[]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/ListGrants {"ref":{"path":"/subdir"},"mdKeys":null} GRANT-ADDED`:   {200, `[ { "stat": true, "move": true, "delete": false } ]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/ListGrants {"ref":{"path":"/subdir"},"mdKeys":null} GRANT-UPDATED`: {200, `[ { "stat": true, "move": true, "delete": true } ]`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/ListRecycle  EMPTY`:   {200, `[]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/ListRecycle  RECYCLE`: {200, `["/subdir"]`, serverStateRecycle},

	`POST /apps/sciencemesh/~einstein/api/storage/ListRevisions {"path":"/versionedFile"} EMPTY`:         {500, `[1]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/storage/ListRevisions {"path":"/versionedFile"} FILE-RESTORED`: {500, `[1, 2]`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/storage/Move {"from":"/subdir","to":"/new_subdir"}`: {200, ``, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/storage/RemoveGrant {"path":"/subdir"} GRANT-ADDED`: {200, ``, serverStateGrantUpdated},

	`POST /apps/sciencemesh/~einstein/api/storage/RestoreRecycleItem null`:                       {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/storage/RestoreRecycleItem {"path":"/subdirRestored"}`: {200, ``, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/storage/RestoreRevision {"path":"/versionedFile"}`: {200, ``, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/storage/SetArbitraryMetadata {"metadata":{"foo":"bar"}}`: {200, ``, serverStateMetadata},

	`POST /apps/sciencemesh/~einstein/api/storage/UnsetArbitraryMetadata {"path":"/subdir"}`: {200, ``, serverStateSubdir},

	`POST /apps/sciencemesh/~einstein/api/storage/UpdateGrant {"path":"/subdir"}`: {200, ``, serverStateGrantUpdated},

	`POST /apps/sciencemesh/~tester/api/storage/GetHome `:    {200, `yes we are`, serverStateHome},
	`POST /apps/sciencemesh/~tester/api/storage/CreateHome `: {201, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/CreateDir {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`:                                                                                                                  {201, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/Delete {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`:                                                                                                                     {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/Move {"from":{"resource_id":{"storage_id":"storage-id-1","opaque_id":"opaque-id-1"},"path":"/some/old/path"},"to":{"resource_id":{"storage_id":"storage-id-2","opaque_id":"opaque-id-2"},"path":"/some/new/path"}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/GetMD {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"},"mdKeys":["val1","val2","val3"]}`:                                                                              {200, `{ "size": 1, "path":"/some/path", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/ListFolder {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"},"mdKeys":["val1","val2","val3"]}`:                                                                         {200, `[{ "size": 1, "path":"/some/path", "metadata": { "foo": "bar" }, "etag": "in-json-etag", "mimetype": "in-json-mimetype" }]`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/InitiateUpload {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"},"uploadLength":12345,"metadata":{"key1":"val1","key2":"val2","key3":"val3"}}`:                         {200, `{ "not":"sure", "what": "should be", "returned": "here" }`, serverStateEmpty},
	`PUT /apps/sciencemesh/~tester/api/storage/Upload/some/file/path.txt shiny!`:                                                                                                                                                                                    {200, ``, serverStateEmpty},
	`GET /apps/sciencemesh/~tester/api/storage/Download/some/file/path.txt `:                                                                                                                                                                                        {200, `the contents of the file`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/ListRevisions {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`:                                                                                                              {200, `[{"key":"version-12", "size": 12345, "mtime": 1234567990, "etag": "deadb00f"}, {"key":"asdf", "size": 1235, "mtime": 1234567890, "etag": "deadbeef"}]`, serverStateEmpty},
	`GET /apps/sciencemesh/~tester/api/storage/DownloadRevision/some%2Frevision/some/file/path.txt `:                                                                                                                                                                {200, `the contents of that revision`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/RestoreRevision {"path":"some/file/path.txt","key":"asdf"}`:                                                                                                                                                         {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/ListRecycle {"path":"/some/file.txt","key":"asdf"}`:                                                                                                                                                                 {200, `[{"key":"deleted-version","size":12345,"deletionTime":1234567890}]`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/RestoreRecycleItem {"key":"asdf","path":"original/location/when/deleted.txt","restoreRef":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"}}`:                         {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/PurgeRecycleItem {"key":"asdf","path":"original/location/when/deleted.txt"}`:                                                                                                                                        {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/EmptyRecycle `:                                                                                                                                                                                                      {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/GetPathByID {"storage_id":"storage-id","opaque_id":"opaque-id"}`:                                                                                                                                                    {200, `the/path/for/that/id.txt`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/AddGrant {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"grant":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/DenyGrant {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/RemoveGrant {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"grant":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/UpdateGrant {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"grant":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/ListGrants {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"}`: {200, `[{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}]`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/GetQuota `:                                                                             {200, `{"maxBytes":456,"maxFiles":123}`, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/CreateReference {"path":"some/file/path.txt","url":"http://bing.com/search?q=dotnet"}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/Shutdown `:                                                                             {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/SetArbitraryMetadata {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"metadata":{"metadata":{"arbi":"trary","meta":"data"}}}`: {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/UnsetArbitraryMetadata {"reference":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"keys":["arbi"]}`:                                      {200, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~tester/api/storage/ListStorageSpaces {"filters":[{"type":3,"Term":{"Owner":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},{"type":2,"Term":{"Id":{"opaque_id":"opaque-id"}}},{"type":4,"Term":{"SpaceType":"home"}}]}`: {200, `	[{"opaque":{"some-opaque-key":"some-opaque-value"},"opaqueId":"storage-space-opaque-id","ownerIdp":"0.0.0.0:19000","ownerOpaqueId":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","rootStorageId":"root-storage-id","rootOpaqueId":"root-opaque-id","name":"My Home Space","quotaMaxBytes":456,"quotaMaxFiles":123,"spaceType":"home","mTimeSeconds":1234567890}]`, serverStateEmpty},
}

// GetNextcloudServerMock returns a handler that pretends to be a remote Nextcloud server
func GetNextcloudServerMock(called *[]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		_, err := io.Copy(buf, r.Body)
		if err != nil {
			panic("Error reading response into buffer")
		}
		var key = fmt.Sprintf("%s %s %s", r.Method, r.URL, buf.String())
		fmt.Printf("Nextcloud Server Mock key components %s %d %s %d %s %d\n", r.Method, len(r.Method), r.URL.String(), len(r.URL.String()), buf.String(), len(buf.String()))
		fmt.Printf("Nextcloud Server Mock key %s\n", key)
		*called = append(*called, key)
		response := responses[key]
		if (response == Response{}) {
			key = fmt.Sprintf("%s %s %s %s", r.Method, r.URL, buf.String(), serverState)
			fmt.Printf("Nextcloud Server Mock key with State %s\n", key)
			// *called = append(*called, key)
			response = responses[key]
		}
		if (response == Response{}) {
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			fmt.Printf("Nextcloud Server Mock key not found! %s\n", key)
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			response = Response{200, fmt.Sprintf("response not defined! %s", key), serverStateEmpty}
		}
		serverState = responses[key].newServerState
		if serverState == `` {
			serverState = serverStateError
		}
		w.WriteHeader(response.code)
		// w.Header().Set("Etag", "mocker-etag")
		_, err = w.Write([]byte(responses[key].body))
		if err != nil {
			panic(err)
		}
	})
}

// TestingHTTPClient thanks to https://itnext.io/how-to-stub-requests-to-remote-hosts-with-go-6c2c1db32bf2
// Ideally, this function would live in tests/helpers, but
// if we put it there, it gets excluded by .dockerignore, and the
// Docker build fails (see https://github.com/cs3org/reva/issues/1999)
// So putting it here for now - open to suggestions if someone knows
// a better way to inject this.
func TestingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
		},
	}

	return cli, s.Close
}
