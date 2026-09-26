(in-package 'sample)
(use-package 'router)

(defendpoint "init" ()
  (route-success ()))

(defendpoint "read" ()
  (route-success (router:loaded-phylum-version)))
