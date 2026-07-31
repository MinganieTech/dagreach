import json
from pathlib import Path

import pytest

from dagreach.adapters import PROFILES, detect_profile, get_profile
from dagreach.adapters.terraform import kind_of, normalise
from dagreach.analysis import impact
from dagreach.cli import EXIT_OK, EXIT_POLICY_FAILED, main
from dagreach.loading import load_graph

FIXTURES = Path(__file__).parent / "fixtures"
TERRAFORM = FIXTURES / "terraform.dot"
MANIFEST = FIXTURES / "dbt-manifest.json"
SBOM = FIXTURES / "sbom.cdx.json"


# -- the registry ----------------------------------------------------------


def test_every_profile_declares_what_it_reads_and_which_way():
    for name, profile in PROFILES.items():
        assert profile.name == name
        assert profile.produced_by and profile.summary
        assert profile.edge_semantics in {"feeds", "depends-on"}


@pytest.mark.parametrize(
    ("path", "expected"),
    [(TERRAFORM, "terraform"), (MANIFEST, "dbt"), (SBOM, "cyclonedx")],
)
def test_producers_are_recognised_from_the_file(path, expected):
    assert detect_profile(path.read_text(encoding="utf-8")).name == expected


def test_an_ordinary_graph_is_recognised_by_nobody():
    assert detect_profile("digraph { a -> b }") is None
    assert get_profile("generic").detect("digraph { a -> b }") is False


def test_detection_is_announced_rather_than_silent():
    graph = load_graph(str(TERRAFORM))
    assert graph.profile == "terraform"
    assert any("recognised from the file itself" in warning for warning in graph.warnings)


def test_an_explicit_profile_is_not_announced():
    graph = load_graph(str(TERRAFORM), profile="terraform")
    assert not any("recognised from the file" in warning for warning in graph.warnings)


def test_an_explicit_semantics_overrides_the_profile_and_says_so():
    graph = load_graph(str(TERRAFORM), profile="terraform", edge_semantics="feeds")
    assert graph.edge_semantics == "feeds"
    assert any("overrides the terraform profile" in warning for warning in graph.warnings)


def test_format_is_ignored_by_a_profile_that_knows_better():
    graph = load_graph(str(MANIFEST), profile="dbt", format="jgf")
    assert any("--format jgf was ignored" in warning for warning in graph.warnings)


# -- terraform -------------------------------------------------------------


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("[root] aws_vpc.main (expand)", "aws_vpc.main"),
        (
            '[root] provider["registry.terraform.io/hashicorp/aws"]',
            'provider["registry.terraform.io/hashicorp/aws"]',
        ),
        ("[root] module.net.aws_subnet.a (close)", "module.net.aws_subnet.a"),
        ("aws_vpc.main", "aws_vpc.main"),
    ],
)
def test_terraform_identifiers_lose_their_decoration(raw, expected):
    assert normalise(raw) == expected


@pytest.mark.parametrize(
    ("node", "group"),
    [
        ("aws_vpc.main", "aws_vpc"),
        ("data.aws_ami.ubuntu", "data"),
        ('provider["registry.terraform.io/hashicorp/aws"]', "provider"),
    ],
)
def test_terraform_groups_by_resource_kind(node, group):
    assert kind_of(node) == group


def test_terraform_impact_runs_the_right_way_without_any_flag():
    graph = load_graph(str(TERRAFORM), profile="terraform")
    assert graph.edge_semantics == "depends-on"
    report = impact(graph, ["aws_vpc.main"])
    assert report.downstream == [
        "aws_instance.web",
        "aws_security_group.web",
        "aws_subnet.main",
    ]
    assert graph.nodes["aws_vpc.main"].attrs["terraform_id"] == "[root] aws_vpc.main (expand)"
    assert graph.nodes["aws_vpc.main"].attrs["group"] == "aws_vpc"


def test_terraform_keeps_full_identifiers_when_stripping_would_collide(tmp_path):
    collision = tmp_path / "collide.dot"
    collision.write_text(
        'digraph { compound = "true"\n newrank = "true"\n'
        ' "[root] aws_vpc.main (expand)" -> "[root] aws_vpc.main (close)" }',
        encoding="utf-8",
    )
    graph = load_graph(str(collision), profile="terraform")
    assert "[root] aws_vpc.main (expand)" in graph.nodes
    assert any("would collide" in warning for warning in graph.warnings)


# -- dbt -------------------------------------------------------------------


def test_dbt_reads_models_sources_tests_and_exposures():
    graph = load_graph(str(MANIFEST), profile="dbt")
    assert graph.name == "acme_analytics"
    assert graph.edge_semantics == "feeds"
    assert graph.node_count == 5
    assert graph.attrs["dbt_version"] == "1.9.2"

    source = graph.nodes["source.acme_analytics.shop.orders"]
    assert source.attrs["group"] == "source"
    model = graph.nodes["model.acme_analytics.fct_orders"]
    assert model.attrs["group"] == "model"
    assert model.attrs["materialized"] == "table"
    assert model.attrs["tags"] == "marts,production"


def test_dbt_impact_runs_downstream_of_a_source():
    graph = load_graph(str(MANIFEST), profile="dbt")
    report = impact(graph, ["source.acme_analytics.shop.orders"])
    assert report.downstream == [
        "model.acme_analytics.stg_orders",
        "model.acme_analytics.fct_orders",
        "test.acme_analytics.not_null_fct_orders_id",
        "exposure.acme_analytics.revenue_dashboard",
    ]


def test_dbt_falls_back_to_depends_on_when_the_manifest_has_no_child_map(tmp_path):
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    del manifest["child_map"]
    path = tmp_path / "manifest.json"
    path.write_text(json.dumps(manifest), encoding="utf-8")

    graph = load_graph(str(path), profile="dbt")
    assert any("depends_on.nodes" in warning for warning in graph.warnings)
    report = impact(graph, ["model.acme_analytics.stg_orders"])
    assert "model.acme_analytics.fct_orders" in report.downstream


# -- cyclonedx -------------------------------------------------------------


def test_cyclonedx_reads_components_and_dependencies():
    graph = load_graph(str(SBOM), profile="cyclonedx")
    assert graph.name == "checkout-service"
    assert graph.edge_semantics == "depends-on"
    assert graph.node_count == 4
    assert graph.attrs["specVersion"] == "1.6"

    qs = graph.nodes["pkg:npm/qs@6.11.0"]
    assert qs.attrs["group"] == "library"
    assert qs.attrs["version"] == "6.11.0"
    assert qs.attrs["licenses"] == "BSD-3-Clause"
    assert graph.nodes["pkg:npm/checkout-service@2.4.0"].attrs["group"] == "root"


def test_cyclonedx_answers_the_supply_chain_question():
    """A library is found vulnerable: what depends on it?"""
    graph = load_graph(str(SBOM), profile="cyclonedx")
    report = impact(graph, ["pkg:npm/qs@6.11.0"])
    assert report.downstream == [
        "pkg:npm/checkout-service@2.4.0",
        "pkg:npm/express@4.19.2",
        "pkg:npm/body-parser@1.20.2",
    ]


def test_an_sbom_without_relationships_is_read_and_flagged(tmp_path):
    path = tmp_path / "flat.cdx.json"
    path.write_text(
        json.dumps(
            {
                "bomFormat": "CycloneDX",
                "specVersion": "1.6",
                "components": [{"bom-ref": "a", "name": "a", "type": "library"}],
            }
        ),
        encoding="utf-8",
    )
    graph = load_graph(str(path), profile="cyclonedx")
    assert graph.node_count == 1
    assert any("no 'dependencies' array" in warning for warning in graph.warnings)


# -- the command line ------------------------------------------------------


def test_profiles_are_listed_with_their_direction(capsys):
    assert main(["profiles"]) == EXIT_OK
    out = capsys.readouterr().out
    assert "terraform" in out and "terraform graph" in out
    assert "cyclonedx" in out and "depends-on" in out
    assert "--edge-semantics overrides it" in out


def test_the_report_names_the_profile_it_applied(capsys):
    assert main(["parse", str(SBOM)]) == EXIT_OK
    out = capsys.readouterr().out
    assert "edges: cyclonedx profile, source depends on target" in out


def test_a_policy_over_an_sbom(capsys):
    code = main(
        [
            "impact",
            str(SBOM),
            "--changed",
            "pkg:npm/qs@6.11.0",
            "--fail-if-reaches",
            "group=root",
            "--explain",
        ]
    )
    assert code == EXIT_POLICY_FAILED
    out = capsys.readouterr().out
    assert "FAIL fail-if-reaches group=root" in out
    assert "pkg:npm/checkout-service@2.4.0" in out


def test_the_profile_travels_into_the_json_report(capsys):
    assert main(["stats", str(MANIFEST), "--json"]) == EXIT_OK
    assert json.loads(capsys.readouterr().out)["profile"] == "dbt"
