(in-package 'sample)
(use-package 'router)

(defendpoint "init" ()
             (route-success ()))

(defendpoint "wrap_all" (msg)
             (handler-bind ((csprng-uninitialized (lambda (c &rest _)
                                                    (route-failure "missing CSPRNG seed"))))
               (let* ([dec (private:mxf-decode msg)]
                      [dec-msg (first dec)]
                      [dec-mxf (second dec)]
                      [new-enc (private:put-mxf "test-key" dec-msg dec-mxf)])
                 (route-success new-enc))))

(defendpoint "wrap_none" (msg)
             (route-success msg))

(defendpoint "wrap_output" (msg)
             (route-success (statedb:get "test-key")))

(defendpoint "wrap_input" (msg)
             (let* ([dec (private:mxf-decode msg)]
                    [dec-msg (first dec)])
               (route-success dec-msg)))
