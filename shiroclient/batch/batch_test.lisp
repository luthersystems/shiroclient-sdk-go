(in-package 'user)
(use-package 'router)
(cc:infof () "init batch.lisp")

(defun schedule (batch-name req when-time)
  (let ([err (batch:schedule-request batch-name req when-time)])
    (if (nil? err)
      (route-success ())
      (route-failure err))))

(defendpoint init ()
  (schedule "init_batch" (sorted-map) (cc:timestamp (cc:now))))

(set 'storage-key-recent-input "RECENT_INPUT")

(batch:handler 'test_batch (lambda (batch-name rep bad)
  (if bad
    (progn
      (cc:storage-put storage-key-recent-input (string:join (list "error:" bad) " "))
      (cc:infof () "error: {}" bad))
    (progn
      (cc:storage-put storage-key-recent-input rep)
      (cc:infof () rep)))
  (route-success ())))

(batch:handler 'init_batch (lambda (batch-name rep bad)
  (cc:infof () (get rep "init message"))
  (route-success ())))

(defendpoint schedule_request (batch_name req when)
  (schedule batch_name req when))

(defendpoint schedule_request_now (batch_name req)
  (cc:infof () "in schedule_request_now")
  (cc:infof () batch_name)
  (schedule batch_name req (cc:timestamp (cc:now))))

(defendpoint set_batching_paused (val)
  (cc:set-app-property "BATCHING_PAUSED" val)
  (route-success ()))

(defendpoint get_recent_input ()
  (route-success (to-string (cc:storage-get storage-key-recent-input))))
