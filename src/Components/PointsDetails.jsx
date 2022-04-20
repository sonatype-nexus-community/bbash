import React, { useState } from "react";
import { Container } from "react-bootstrap";
import BootstrapTable from "react-bootstrap-table-next";
import paginationFactory from "react-bootstrap-table2-paginator";

export default function PointsDetails() {
  const [tableData, setTableData] = useState([
    {
      id: "0",
      name: "slayer321",
      points: "237",
    },
    {
      id: "1",
      name: "100mik",
      points: "168",
    },
    {
      id: "3",
      name: "shatakshi0805",
      points: "107",
    },
    {
      id: "4",
      name: "marcelmue",
      points: "63",
    },
    {
      id: "5",
      name: "abhishek-kumar09",
      points: "22",
    },
    {
      id: "6",
      name: "anuragpaliwal80",
      points: "14",
    },
    {
      id: "7",
      name: "slayer321",
      points: "237",
    },
    {
      id: "8",
      name: "100mik",
      points: "168",
    },
    {
      id: "9",
      name: "shatakshi0805",
      points: "107",
    },
    {
      id: "10",
      name: "marcelmue",
      points: "63",
    },
    {
      id: "11",
      name: "abhishek-kumar09",
      points: "22",
    },
    {
      id: "12",
      name: "anuragpaliwal80",
      points: "14",
    },
  ]);
  const columns = [
    {
      dataField: "name",
      text: "GITHUB ID",
      sort: true,
      classes: 'w-50'
    },
    {
      dataField: "points",
      text: "POINTS",
      sort: true,
      classes: 'w-50'
    },
  ];
  return (
    <div className="points_wrapper">
      <Container>
        <div className="points_wrapper_inner">
          <h3 className="mb-4">Take Me To Your Leaderboard</h3>
          <BootstrapTable
          bootstrap4
          keyField="name"
          data={tableData}
          columns={columns}
          pagination={paginationFactory({sizePerPage: 4, hideSizePerPage: true, nextPageText: 'See More', prePageText: 'Previous',})}
          />
        </div>
      </Container>
    </div>
  );
}
