(in-package 'sample)
(use-package 'router)

; chaincode metadata
(set 'version "18.09.21")
(set 'service-name "sample")

(defendpoint "init" ()
  (route-success ()))

(defendpoint "healthcheck" ()
  (route-success
   (sorted-map "reports"
               (vector (sorted-map
                        "status"          "UP"
                        "service_version" version
                        "service_name"    service-name
                        "timestamp"       (cc:timestamp (cc:now)))))))

(defendpoint "write" (val)
  (statedb:put "testkey" val))

(defendpoint "read" ()
  (route-success (statedb:get "testkey")))

; writes, then fails: the write must not commit.
(defendpoint "write-then-fail" (val)
  (statedb:put "testkey" val)
  (route-failure "boom"))

; forces its transaction not to commit.
(defendpoint "no-commit" ()
  (cc:force-no-commit-tx)
  (route-success ()))
