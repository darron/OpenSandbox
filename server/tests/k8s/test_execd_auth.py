from opensandbox_server.api.proxy import _filter_proxy_headers
from opensandbox_server.api.schema import Endpoint, ImageSpec
from opensandbox_server.services.constants import (
    EXECD_ACCESS_TOKEN_HEADER,
    SANDBOX_EXECD_ACCESS_TOKEN_METADATA_KEY,
)
from opensandbox_server.services.k8s.agent_sandbox_provider import AgentSandboxProvider
from opensandbox_server.services.k8s.endpoint_resolver import _attach_execd_auth_headers
from tests.k8s.test_agent_sandbox_provider import _app_config


def test_workload_credentials_override_untrusted_env_and_are_unique(mock_k8s_client):
    provider = AgentSandboxProvider(mock_k8s_client, _app_config())
    mock_k8s_client.create_custom_object.return_value = {
        "metadata": {"name": "test", "uid": "test-uid"}
    }
    tokens = []
    for sandbox_id in ["one", "two"]:
        provider.create_workload(
            sandbox_id=sandbox_id,
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={"EXECD_ACCESS_TOKEN": "attacker", "EXECD_RUNTIME_INIT": "true"},
            annotations={SANDBOX_EXECD_ACCESS_TOKEN_METADATA_KEY: "attacker"},
            resource_limits={"cpu": "1", "memory": "1Gi"},
            labels={"opensandbox.io/id": sandbox_id},
            expires_at=None,
            execd_image="execd:test",
        )
        body = mock_k8s_client.create_custom_object.call_args.kwargs["body"]
        token = body["metadata"]["annotations"][SANDBOX_EXECD_ACCESS_TOKEN_METADATA_KEY]
        assert len(token) >= 32 and token != "attacker"
        env = body["spec"]["podTemplate"]["spec"]["containers"][0]["env"]
        assert [e for e in env if e["name"] == "EXECD_ACCESS_TOKEN"] == [
            {"name": "EXECD_ACCESS_TOKEN", "value": token}
        ]
        assert [e for e in env if e["name"] == "EXECD_RUNTIME_INIT"] == [
            {"name": "EXECD_RUNTIME_INIT", "value": "false"}
        ]
        endpoint = Endpoint(endpoint="10.0.0.1:44772")
        _attach_execd_auth_headers(endpoint, body, 44772)
        assert endpoint.headers == {EXECD_ACCESS_TOKEN_HEADER: token}
        tokens.append(token)
    assert tokens[0] != tokens[1]


def test_execd_token_is_not_sent_to_application_ports():
    workload = {"metadata": {"annotations": {SANDBOX_EXECD_ACCESS_TOKEN_METADATA_KEY: "secret"}}}
    endpoint = Endpoint(endpoint="10.0.0.1:8080", headers={"other": "value"})
    _attach_execd_auth_headers(endpoint, workload, 8080)
    assert endpoint.headers == {"other": "value"}


def test_proxy_replaces_every_inbound_token_spelling():
    inbound = {
        "x-execd-access-token": "attacker",
        "X-ExEcD-AcCeSs-ToKeN": "attacker-two",
        "Content-Type": "application/json",
    }
    outgoing = _filter_proxy_headers(inbound, {EXECD_ACCESS_TOKEN_HEADER: "trusted"})
    assert outgoing == {
        "Content-Type": "application/json", EXECD_ACCESS_TOKEN_HEADER: "trusted"
    }
