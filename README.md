# AEAPI

Expose JSON endpoints for internal app engine 1st gen features that hasn't been released as a standalone product.

This will mostly be used for appengine search api. Plus a structure starter for golang backend for next gen app engine apps.

## Run Locally & Deploy

    dev_appserver.py app.yaml
  
    gcloud app deploy app.yaml --project=project-id --

## Endpoints

    POST /memcache
    Content-Type: application/json

    {
        "Items": [
            {"Key": "a", "Value": "YWFhYWFhYQ=="},
            {"Key": "b", "Value": "YWFhYWFhYQ=="}
        ]
    }    

    GET /memcache?key=a&key=b
    Response:
    {
        "a": {
            "Key": "a",
            "Value": "YWFhYWFhYQ==",
            "Object": null,
            "Flags": 0,
            "Expiration": 0
        },
        "b": {
            "Key": "b",
            "Value": "YWFhYWFhYQ==",
            "Object": null,
            "Flags": 0,
            "Expiration": 0
        }
    }
