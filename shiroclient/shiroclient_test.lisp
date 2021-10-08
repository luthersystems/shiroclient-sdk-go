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
